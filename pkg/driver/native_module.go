package driver

import (
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/nooga/paserati/pkg/lexer"
	"github.com/nooga/paserati/pkg/modules"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

const debugNativeModules = false // Enable debug output for native modules

// ModuleBuilder provides the declarative API for building native modules
// This works like builtin initializers, directly creating types and runtime values
type ModuleBuilder struct {
	// Type information (populated directly)
	exports map[string]types.Type

	// Runtime values (populated directly)
	values map[string]vm.Value

	// VM instance for creating runtime objects
	vm *vm.VM

	// Default export (applied after the builder callback returns)
	hasDefault   bool
	defaultValue interface{} // nil means "namespace of named exports"
}

// NamespaceBuilder provides API for building namespaces within modules
type NamespaceBuilder struct {
	// Type information
	exports map[string]types.Type

	// Runtime values
	values map[string]vm.Value

	// VM instance for creating runtime objects
	vm *vm.VM
}

// NativeModule represents a module declared in Go code
type NativeModule struct {
	name        string
	builder     func(*ModuleBuilder)
	initialized bool
	exports     map[string]types.Type // Type information
	values      map[string]vm.Value   // Runtime values
	mutex       sync.Once
}

// ValueConverter handles conversion between Go values and VM values
type ValueConverter struct {
	vm *vm.VM
}

// TypeGenerator uses reflection to generate TypeScript types from Go types
type TypeGenerator struct {
	typeCache map[reflect.Type]types.Type
}

// NativeModuleResolver resolves native modules declared in Go
type NativeModuleResolver struct {
	modules  map[string]*NativeModule
	priority int
}

// Const adds a constant to the module
func (m *ModuleBuilder) Const(name string, value interface{}) *ModuleBuilder {
	// Create TypeScript type directly
	tsType := m.goValueToTSType(value)
	m.exports[name] = tsType

	// Create runtime value directly
	vmValue := m.goValueToVM(value)
	m.values[name] = vmValue

	return m
}

// Let adds a variable to the module (same as Const for now)
func (m *ModuleBuilder) Let(name string, value interface{}) *ModuleBuilder {
	return m.Const(name, value)
}

// Var adds a variable to the module (same as Const for now)
func (m *ModuleBuilder) Var(name string, value interface{}) *ModuleBuilder {
	return m.Const(name, value)
}

// Function adds a function to the module
func (m *ModuleBuilder) Function(name string, fn interface{}) *ModuleBuilder {
	// Create TypeScript function type directly
	tsType := m.goFunctionToTSType(fn)
	m.exports[name] = tsType

	// Create runtime function directly
	vmValue := m.goFunctionToVM(fn)
	m.values[name] = vmValue

	return m
}

// AsyncFunction adds a function whose JS return value is a Promise.
// The Go implementation still runs synchronously on the caller's thread
// (do not use this for real I/O — copy the fetch BeginExternalOp pattern).
func (m *ModuleBuilder) AsyncFunction(name string, fn interface{}) *ModuleBuilder {
	m.exports[name] = goAsyncFunctionToTSType(fn)
	m.values[name] = wrapNativeAsAsync(m.vm, name, goFunctionToVM(fn))
	return m
}

func goAsyncFunctionToTSType(fn interface{}) types.Type {
	fnType := reflect.TypeOf(fn)
	if fnType.Kind() != reflect.Func {
		return types.Any
	}
	params := make([]types.Type, fnType.NumIn())
	for i := 0; i < fnType.NumIn(); i++ {
		params[i] = goTypeToTSType(fnType.In(i))
	}
	var returnType types.Type = types.Void
	if fnType.NumOut() > 0 {
		returnType = goTypeToTSType(fnType.Out(0))
	}
	if returnType == nil {
		returnType = types.Any
	}
	promiseRet := types.NewInstantiatedType(types.PromiseGeneric, []types.Type{returnType})
	return types.NewSimpleFunction(params, promiseRet)
}

func wrapNativeAsAsync(vmInst *vm.VM, name string, inner vm.Value) vm.Value {
	if vmInst == nil || !inner.IsCallable() {
		return inner
	}
	arity := 0
	variadic := true
	if inner.Type() == vm.TypeNativeFunction {
		nf := inner.AsNativeFunction()
		arity = nf.Arity
		variadic = nf.Variadic
	}
	return vm.NewNativeFunction(arity, variadic, name, func(args []vm.Value) (vm.Value, error) {
		result, err := vmInst.Call(inner, vm.Undefined, args)
		if err != nil {
			// Reject with the real thrown value when the inner function threw
			// one (#147). Flattening it to vm.NewString(err.Error()) used to
			// drop everything a host had built onto its Error object - .code,
			// .errno, .syscall - so an async native API's rejection reason
			// arrived in .catch() as a bare string while its synchronous
			// counterpart, built the identical way, threw the real object.
			return vmInst.NewRejectedPromise(vmInst.ExceptionValueFromError(err)), nil
		}
		return vmInst.NewResolvedPromise(result), nil
	})
}

// Constructor adds a constructor function to the module
func (m *ModuleBuilder) Constructor(name string, fn interface{}) *ModuleBuilder {
	return m.Function(name, fn)
}

// Class adds a class/struct to the module with a custom constructor function
// Usage: m.Class("Point", (*Point)(nil), func(x, y float64) *Point { return &Point{X: x, Y: y} })
func (m *ModuleBuilder) Class(name string, goStruct interface{}, constructor interface{}) *ModuleBuilder {
	// Create a constructor function type from the constructor function
	constructorType := m.goFunctionToTSType(constructor)

	// Export the constructor as the class
	m.exports[name] = constructorType

	// Create a constructor function that properly creates instances with prototypes
	constructorValue := m.createClassConstructor(name, goStruct, constructor)
	m.values[name] = constructorValue

	return m
}

// Namespace creates a namespace within the module
func (m *ModuleBuilder) Namespace(name string, builder func(ns *NamespaceBuilder)) *ModuleBuilder {
	ns := &NamespaceBuilder{
		exports: make(map[string]types.Type),
		values:  make(map[string]vm.Value),
		vm:      m.vm,
	}

	builder(ns)

	// Create namespace type directly
	nsType := types.NewObjectType()
	for propName, propType := range ns.exports {
		nsType = nsType.WithProperty(propName, propType)
	}
	m.exports[name] = nsType

	// Create namespace runtime object directly
	nsObj := vm.NewObject(vm.Undefined)
	nsObjPtr := nsObj.AsPlainObject()
	for propName, propValue := range ns.values {
		nsObjPtr.SetOwn(propName, propValue)
	}
	m.values[name] = nsObj

	return m
}

// Interface adds a TypeScript interface (TODO: implement)
func (m *ModuleBuilder) Interface(name string, fields map[string]interface{}) *ModuleBuilder {
	// TODO: implement interface generation
	return m
}

// Type adds a type alias (TODO: implement)
func (m *ModuleBuilder) Type(name string, typedef interface{}) *ModuleBuilder {
	// TODO: implement type alias generation
	return m
}

// Default sets the ESM default export.
//
// Passing nil makes the default a namespace object containing every named
// export (Node CJS interop: `import fs from "fs"`). Named exports must be
// registered before Default(nil); it is applied when the builder returns.
func (m *ModuleBuilder) Default(value interface{}) *ModuleBuilder {
	m.hasDefault = true
	m.defaultValue = value
	return m
}

func (m *ModuleBuilder) applyDefaultExport() {
	if !m.hasDefault {
		return
	}

	if m.defaultValue == nil {
		proto := vm.Undefined
		if m.vm != nil {
			proto = m.vm.ObjectPrototype
		}
		nsType := types.NewObjectType()
		nsObj := vm.NewObject(proto).AsPlainObject()
		for name, typ := range m.exports {
			if name == "default" {
				continue
			}
			nsType = nsType.WithProperty(name, typ)
			if val, ok := m.values[name]; ok {
				nsObj.SetOwn(name, val)
			}
		}
		m.exports["default"] = nsType
		m.values["default"] = vm.NewValueFromPlainObject(nsObj)
		return
	}

	if reflect.TypeOf(m.defaultValue) != nil && reflect.TypeOf(m.defaultValue).Kind() == reflect.Func {
		m.exports["default"] = m.goFunctionToTSType(m.defaultValue)
		m.values["default"] = m.goFunctionToVM(m.defaultValue)
		return
	}

	m.exports["default"] = m.goValueToTSType(m.defaultValue)
	m.values["default"] = m.goValueToVM(m.defaultValue)
}

// NamespaceBuilder methods (similar to ModuleBuilder)

func (ns *NamespaceBuilder) Const(name string, value interface{}) *NamespaceBuilder {
	// Create TypeScript type directly
	tsType := goValueToTSType(value)
	ns.exports[name] = tsType

	// Create runtime value directly
	vmValue := goValueToVM(value)
	ns.values[name] = vmValue

	return ns
}

func (ns *NamespaceBuilder) Function(name string, fn interface{}) *NamespaceBuilder {
	// Create TypeScript function type directly
	tsType := goFunctionToTSType(fn)
	ns.exports[name] = tsType

	// Create runtime function directly
	vmValue := goFunctionToVM(fn)
	ns.values[name] = vmValue

	return ns
}

// Helper methods for ModuleBuilder

// goValueToTSType converts a Go value to a TypeScript type
func (m *ModuleBuilder) goValueToTSType(value interface{}) types.Type {
	return goValueToTSType(value)
}

// goValueToVM converts a Go value to a VM value
func (m *ModuleBuilder) goValueToVM(value interface{}) vm.Value {
	return goValueToVM(value)
}

// goFunctionToTSType converts a Go function to a TypeScript function type
func (m *ModuleBuilder) goFunctionToTSType(fn interface{}) types.Type {
	return goFunctionToTSType(fn)
}

// goFunctionToVM converts a Go function to a VM native function
func (m *ModuleBuilder) goFunctionToVM(fn interface{}) vm.Value {
	return goFunctionToVM(fn)
}

// createClassConstructor creates a JavaScript-style constructor function that:
// 1. Can be called with 'new' to create instances
// 2. Binds methods from the Go struct to the created instance
// 3. Sets up proper prototype chain
func (m *ModuleBuilder) createClassConstructor(name string, goStruct interface{}, constructor interface{}) vm.Value {
	constructorFn := reflect.ValueOf(constructor)
	constructorType := reflect.TypeOf(constructor)
	structType := reflect.TypeOf(goStruct).Elem() // Remove pointer to get struct type

	if constructorType.Kind() != reflect.Func {
		return vm.Undefined
	}

	// A shared prototype object for instances to point at, so `instanceof`
	// has something to walk to instead of throwing "Function has non-object
	// prototype in instanceof check" (paserati#201). bindStructMethods binds
	// methods directly onto each *instance* rather than this prototype, so
	// it only needs to exist and carry `constructor` - nothing else reads it.
	protoParent := vm.Undefined
	if m.vm != nil {
		protoParent = m.vm.ObjectPrototype
	}
	classPrototype := vm.NewObject(protoParent).AsPlainObject()
	classPrototypeVal := vm.NewValueFromPlainObject(classPrototype)

	constructorValue := vm.NewConstructorWithProps(constructorType.NumIn(), constructorType.IsVariadic(), name, func(args []vm.Value) (vm.Value, error) {
		// Convert VM values to Go values for constructor call. Capped at
		// constructorType.NumIn() (paserati#278): a non-variadic Go function
		// called with MORE JS args than it declares (routine - e.g. any
		// generic JS machinery that always calls back with a fixed argument
		// count, regardless of what the callee actually wants) must have
		// those extras silently dropped, matching real JS semantics. Sizing
		// goArgs to len(args) and only filling indices < NumIn() left the
		// tail as unset reflect.Value{} zero values and handed
		// reflect.Value.Call a slice longer than the function's declared
		// arity - which panics outright rather than ignoring the extras.
		goArgs := make([]reflect.Value, 0, constructorType.NumIn())
		for i := 0; i < len(args) && i < constructorType.NumIn(); i++ {
			goArgs = append(goArgs, vmValueToReflectValue(args[i], constructorType.In(i)))
		}

		// Add missing arguments as zero values if constructor expects more
		for i := len(args); i < constructorType.NumIn(); i++ {
			goArgs = append(goArgs, reflect.Zero(constructorType.In(i)))
		}

		// Call the Go constructor function
		results := constructorFn.Call(goArgs)
		if len(results) == 0 {
			return vm.Undefined, nil
		}

		// Handle the common (value, error) constructor shape (paserati#167):
		// mirrors goFunctionToVM's own len(results) == 2 error check above -
		// without this, a non-nil error here was silently discarded and
		// `new X(...)` evaluated to undefined instead of throwing.
		if len(results) == 2 && results[1].Type().Implements(reflect.TypeOf((*error)(nil)).Elem()) {
			if !results[1].IsNil() {
				errVal := results[1].Interface().(error)
				return vm.Undefined, errVal
			}
		}

		// Get the created Go instance
		goInstance := results[0]
		if !goInstance.IsValid() || goInstance.IsNil() {
			return vm.Undefined, nil
		}

		// Create a VM object to represent the instance, chained to the
		// class's shared prototype so `instance instanceof Class` resolves.
		instance := vm.NewObject(classPrototypeVal)
		instanceObj := instance.AsPlainObject()

		// Bind all methods from the Go struct to the VM object
		m.bindStructMethods(instanceObj, goInstance, structType)

		// Store the Go instance as a hidden property for method calls
		// This is a simple approach - in a full implementation, you'd use a more sophisticated storage
		// For now, we'll just bind methods directly without storing the Go instance

		return instance, nil
	})

	if constructorValue.Type() == vm.TypeNativeFunctionWithProps {
		constructorValue.AsNativeFunctionWithProps().Properties.DefineFixedProperty("prototype", classPrototypeVal)
	}
	classPrototype.SetOwnNonEnumerable("constructor", constructorValue)

	return constructorValue
}

// bindStructMethods binds all exported methods from a Go struct to a VM object
func (m *ModuleBuilder) bindStructMethods(vmObj *vm.PlainObject, goInstance reflect.Value, structType reflect.Type) {
	// First, bind struct fields as properties (respecting JSON tags)
	m.bindStructFields(vmObj, goInstance, structType)

	// Then, bind methods
	// Get the pointer type for method lookup
	ptrType := reflect.PointerTo(structType)

	// Iterate through all methods of the pointer type
	for i := 0; i < ptrType.NumMethod(); i++ {
		method := ptrType.Method(i)

		// Skip unexported methods
		if !method.IsExported() {
			continue
		}

		methodName := method.Name
		methodFunc := goInstance.MethodByName(methodName)

		if !methodFunc.IsValid() {
			continue
		}

		// Create a VM function that calls the Go method
		vmMethod := m.createBoundMethod(methodFunc)

		// Convert method name to camelCase for JavaScript compatibility
		jsMethodName := strings.ToLower(methodName[:1]) + methodName[1:]
		vmObj.SetOwn(jsMethodName, vmMethod)
	}
}

// createBoundMethod creates a VM function that calls a bound Go method
func (m *ModuleBuilder) createBoundMethod(methodFunc reflect.Value) vm.Value {
	methodType := methodFunc.Type()

	return vm.NewNativeFunction(methodType.NumIn(), methodType.IsVariadic(), "bound_method", func(args []vm.Value) (vm.Value, error) {
		// Convert VM values to Go values for method call. Capped at
		// methodType.NumIn() (paserati#278) - see the identical comment in
		// createClassConstructor.
		goArgs := make([]reflect.Value, 0, methodType.NumIn())
		for i := 0; i < len(args) && i < methodType.NumIn(); i++ {
			goArgs = append(goArgs, vmValueToReflectValue(args[i], methodType.In(i)))
		}

		// Add missing arguments as zero values if method expects more
		for i := len(args); i < methodType.NumIn(); i++ {
			goArgs = append(goArgs, reflect.Zero(methodType.In(i)))
		}

		// Call the Go method
		results := methodFunc.Call(goArgs)

		// Handle the common (value, error) method shape (paserati#221):
		// mirrors createClassConstructor's own len(results) == 2 error check -
		// without this, a non-nil error here was silently discarded and the
		// call evaluated to a value instead of throwing.
		if len(results) == 2 && results[1].Type().Implements(reflect.TypeOf((*error)(nil)).Elem()) {
			if !results[1].IsNil() {
				errVal := results[1].Interface().(error)
				return vm.Undefined, errVal
			}
		}

		// Convert result back to VM value
		if len(results) > 0 {
			return reflectValueToVM(results[0]), nil
		}

		return vm.Undefined, nil
	})
}

// bindStructFields binds struct fields as properties, respecting JSON tags
func (m *ModuleBuilder) bindStructFields(vmObj *vm.PlainObject, goInstance reflect.Value, structType reflect.Type) {
	// Iterate through all fields of the struct
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get the property name from JSON tag or field name
		propName := m.getJSONPropertyName(field)
		if propName == "" || propName == "-" {
			continue // Skip fields marked with json:"-" or empty names
		}

		// Get the field value from the Go instance
		// If goInstance is a pointer, dereference it to access fields
		structValue := goInstance
		if goInstance.Kind() == reflect.Ptr {
			structValue = goInstance.Elem()
		}

		fieldValue := structValue.FieldByName(field.Name)
		if !fieldValue.IsValid() {
			continue
		}

		// Create getter and setter for this property
		m.createFieldAccessors(vmObj, propName, fieldValue, field.Type)
	}
}

// getJSONPropertyName extracts the property name from JSON tags or uses the field name
func (m *ModuleBuilder) getJSONPropertyName(field reflect.StructField) string {
	// Check for json tag
	if jsonTag := field.Tag.Get("json"); jsonTag != "" {
		// Parse the JSON tag (format: "name,options")
		parts := strings.Split(jsonTag, ",")
		name := strings.TrimSpace(parts[0])

		// Handle special cases
		if name == "-" {
			return "" // Skip this field
		}
		if name == "" {
			return field.Name // Use field name if tag is empty
		}
		return name
	}

	// No JSON tag, use the field name
	return field.Name
}

// createFieldAccessors creates getter/setter accessors for a struct field
func (m *ModuleBuilder) createFieldAccessors(vmObj *vm.PlainObject, propName string, fieldValue reflect.Value, fieldType reflect.Type) {
	// For now, create a simple property with the current value
	// In a full implementation, you'd create actual getter/setter descriptors
	vmValue := reflectValueToVM(fieldValue)
	vmObj.SetOwn(propName, vmValue)

	// TODO: Implement proper getter/setter descriptors that can:
	// 1. Read from the Go struct field when accessed
	// 2. Write back to the Go struct field when assigned
	// This would require VM support for property descriptors
}

// Global helper functions for type/value conversion

// goValueToTSType converts a Go value to a TypeScript type
func goValueToTSType(value interface{}) types.Type {
	switch value.(type) {
	case string:
		return types.String
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return types.Number
	case float32, float64:
		return types.Number
	case bool:
		return types.Boolean
	case nil:
		return types.Null
	default:
		// For complex types, return Any for now
		return types.Any
	}
}

// goValueToVM converts a Go value to a VM value
func goValueToVM(value interface{}) vm.Value {
	switch v := value.(type) {
	case string:
		return vm.NewString(v)
	case int:
		return vm.NumberValue(float64(v))
	case int64:
		return vm.NumberValue(float64(v))
	case float32:
		return vm.NumberValue(float64(v))
	case float64:
		return vm.NumberValue(v)
	case bool:
		return vm.BooleanValue(v)
	case nil:
		return vm.Null
	default:
		return vm.Undefined
	}
}

// goFunctionToTSType converts a Go function to a TypeScript function type using reflection
func goFunctionToTSType(fn interface{}) types.Type {
	fnType := reflect.TypeOf(fn)
	if fnType.Kind() != reflect.Func {
		return types.Any
	}

	// Build parameter types
	params := make([]types.Type, fnType.NumIn())
	for i := 0; i < fnType.NumIn(); i++ {
		params[i] = goTypeToTSType(fnType.In(i))
	}

	// Build return type
	var returnType types.Type = types.Void
	if fnType.NumOut() > 0 {
		returnType = goTypeToTSType(fnType.Out(0))
	}

	return types.NewSimpleFunction(params, returnType)
}

// goFunctionToVM converts a Go function to a VM native function using reflection
func goFunctionToVM(fn interface{}) vm.Value {
	fnValue := reflect.ValueOf(fn)
	fnType := reflect.TypeOf(fn)

	if fnType.Kind() != reflect.Func {
		return vm.Undefined
	}

	// For variadic functions, the minimum required args is NumIn() - 1
	minArgs := fnType.NumIn()
	if fnType.IsVariadic() && minArgs > 0 {
		minArgs--
	}

	return vm.NewNativeFunction(minArgs, fnType.IsVariadic(), "native_function", func(args []vm.Value) (vm.Value, error) {
		// Handle variadic functions specially
		if fnType.IsVariadic() {
			// For variadic functions, we need to handle the last parameter differently
			normalArgCount := fnType.NumIn() - 1
			goArgs := make([]reflect.Value, 0, len(args))

			// Convert normal arguments
			for i := 0; i < normalArgCount && i < len(args); i++ {
				goArgs = append(goArgs, vmValueToReflectValue(args[i], fnType.In(i)))
			}

			// Add zero values for missing normal arguments
			for i := len(args); i < normalArgCount; i++ {
				goArgs = append(goArgs, reflect.Zero(fnType.In(i)))
			}

			// Collect remaining args for variadic parameter
			if len(args) > normalArgCount {
				// The variadic parameter type is a slice
				variadicType := fnType.In(normalArgCount).Elem()
				for i := normalArgCount; i < len(args); i++ {
					goArgs = append(goArgs, vmValueToReflectValue(args[i], variadicType))
				}
			}

			// Call the function - use Call for variadic functions too
			// CallSlice requires the variadic args to be passed as a slice value
			results := fnValue.Call(goArgs)

			// Handle return values
			if len(results) == 2 {
				// Check if the second value is an error
				if results[1].Type().Implements(reflect.TypeOf((*error)(nil)).Elem()) {
					if !results[1].IsNil() {
						// Error occurred
						errVal := results[1].Interface().(error)
						return vm.Undefined, errVal
					}
				}
				// No error, return the first value
				return reflectValueToVM(results[0]), nil
			} else if len(results) == 1 {
				// Single return value
				return reflectValueToVM(results[0]), nil
			}

			return vm.Undefined, nil
		}

		// Non-variadic function handling (original code). Capped at
		// fnType.NumIn() (paserati#278) - see the identical comment in
		// createClassConstructor.
		goArgs := make([]reflect.Value, 0, fnType.NumIn())
		for i := 0; i < len(args) && i < fnType.NumIn(); i++ {
			goArgs = append(goArgs, vmValueToReflectValue(args[i], fnType.In(i)))
		}

		// Add missing arguments as zero values if function expects more
		for i := len(args); i < fnType.NumIn(); i++ {
			goArgs = append(goArgs, reflect.Zero(fnType.In(i)))
		}

		// Call the Go function
		results := fnValue.Call(goArgs)

		// Handle multiple return values (common pattern: (result, error))
		if len(results) == 2 {
			// Check if the second value is an error
			if results[1].Type().Implements(reflect.TypeOf((*error)(nil)).Elem()) {
				if !results[1].IsNil() {
					// Error occurred
					errVal := results[1].Interface().(error)
					return vm.Undefined, errVal
				}
			}
			// No error, return the first value
			return reflectValueToVM(results[0]), nil
		} else if len(results) == 1 {
			// Single return value
			return reflectValueToVM(results[0]), nil
		}

		return vm.Undefined, nil
	})
}

// goTypeToTSType maps Go types to TypeScript types
func goTypeToTSType(t reflect.Type) types.Type {
	switch t.Kind() {
	case reflect.String:
		return types.String
	case reflect.Float64, reflect.Float32:
		return types.Number
	case reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8,
		reflect.Uint, reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8:
		return types.Number
	case reflect.Bool:
		return types.Boolean
	case reflect.Map:
		return mapTypeToObjectType(t, goTypeToTSType(t.Elem()))
	case reflect.Slice:
		// Handle slices, particularly []byte -> Uint8Array
		if t.Elem().Kind() == reflect.Uint8 {
			// Return Any type for Uint8Array since we don't have direct access to the built-in type
			// The runtime will create a proper Uint8Array instance
			return types.Any
		}
		// For other slice types, return array type
		elemType := goTypeToTSType(t.Elem())
		return &types.ArrayType{ElementType: elemType}
	default:
		return types.Any
	}
}

func mapTypeToObjectType(t reflect.Type, valueType types.Type) types.Type {
	if t.Key().Kind() != reflect.String {
		return types.Any
	}
	if valueType == nil {
		valueType = types.Any
	}
	return &types.ObjectType{
		Properties: make(map[string]types.Type),
		IndexSignatures: []*types.IndexSignature{
			{KeyType: types.String, ValueType: valueType},
		},
	}
}

// vmValueToReflectValue converts a VM value to a reflect.Value for function calls
func vmValueToReflectValue(vmVal vm.Value, targetType reflect.Type) reflect.Value {
	// A Go parameter typed vm.Value itself wants the raw argument, untouched.
	// Mirror reflectValueToVM's symmetric passthrough on the return side.
	if targetType == reflect.TypeOf(vm.Value{}) {
		return reflect.ValueOf(vmVal)
	}
	switch targetType.Kind() {
	case reflect.String:
		if vmVal.IsString() {
			return reflect.ValueOf(vmVal.AsString())
		}
		return reflect.ValueOf(vmVal.ToString())
	case reflect.Float64:
		if vmVal.IsNumber() {
			return reflect.ValueOf(vmVal.ToFloat())
		}
		return reflect.ValueOf(0.0)
	case reflect.Float32:
		if vmVal.IsNumber() {
			return reflect.ValueOf(float32(vmVal.ToFloat()))
		}
		return reflect.ValueOf(float32(0.0))
	case reflect.Int, reflect.Int64:
		if vmVal.IsNumber() {
			return reflect.ValueOf(int64(vmVal.ToFloat())).Convert(targetType)
		}
		return reflect.Zero(targetType)
	case reflect.Bool:
		if vmVal.IsBoolean() {
			return reflect.ValueOf(vmVal.AsBoolean())
		}
		return reflect.ValueOf(false)
	case reflect.Map:
		// Convert VM object to Go map
		if vmVal.IsObject() || vmVal.IsDictObject() {
			// Create a new map of the target type
			mapType := targetType
			newMap := reflect.MakeMap(mapType)

			// Get the object
			var obj interface {
				OwnKeys() []string
				GetOwn(string) (vm.Value, bool)
			}
			if vmVal.IsObject() {
				obj = vmVal.AsPlainObject()
			} else if vmVal.IsDictObject() {
				obj = vmVal.AsDictObject()
			}

			if obj != nil {
				// Copy all properties
				for _, key := range obj.OwnKeys() {
					if val, ok := obj.GetOwn(key); ok {
						// Convert the key
						keyVal := reflect.ValueOf(key)
						// Convert the value recursively
						elemType := mapType.Elem()
						valVal := vmValueToReflectValue(val, elemType)
						if valVal.IsValid() {
							newMap.SetMapIndex(keyVal, valVal)
						}
					}
				}
			}

			return newMap
		}
		return reflect.Zero(targetType)
	case reflect.Interface:
		// For interface{}, we need to convert to a concrete Go type
		switch {
		case vmVal.IsString():
			return reflect.ValueOf(vmVal.AsString())
		case vmVal.IsNumber():
			return reflect.ValueOf(vmVal.ToFloat())
		case vmVal.IsBoolean():
			return reflect.ValueOf(vmVal.AsBoolean())
		case vmVal.Type() == vm.TypeNull:
			return reflect.Zero(targetType)
		case vmVal.IsObject() || vmVal.IsDictObject():
			// Convert to map[string]interface{}
			result := make(map[string]interface{})

			var obj interface {
				OwnKeys() []string
				GetOwn(string) (vm.Value, bool)
			}
			if vmVal.IsObject() {
				obj = vmVal.AsPlainObject()
			} else if vmVal.IsDictObject() {
				obj = vmVal.AsDictObject()
			}

			if obj != nil {
				for _, key := range obj.OwnKeys() {
					if val, ok := obj.GetOwn(key); ok {
						// Recursively convert value
						result[key] = vmValueToInterface(val)
					}
				}
			}

			return reflect.ValueOf(result)
		default:
			return reflect.Zero(targetType)
		}
	default:
		return reflect.Zero(targetType)
	}
}

// vmValueToInterface converts a VM value to a Go interface{}
func vmValueToInterface(vmVal vm.Value) interface{} {
	switch {
	case vmVal.IsString():
		return vmVal.AsString()
	case vmVal.IsNumber():
		return vmVal.ToFloat()
	case vmVal.IsBoolean():
		return vmVal.AsBoolean()
	case vmVal.Type() == vm.TypeNull:
		return nil
	case vmVal.IsObject() || vmVal.IsDictObject():
		// Convert to map[string]interface{}
		result := make(map[string]interface{})

		var obj interface {
			OwnKeys() []string
			GetOwn(string) (vm.Value, bool)
		}
		if vmVal.IsObject() {
			obj = vmVal.AsPlainObject()
		} else if vmVal.IsDictObject() {
			obj = vmVal.AsDictObject()
		}

		if obj != nil {
			for _, key := range obj.OwnKeys() {
				if val, ok := obj.GetOwn(key); ok {
					// Recursively convert value
					result[key] = vmValueToInterface(val)
				}
			}
		}

		return result
	default:
		return nil
	}
}

// reflectValueToVM converts a reflect.Value to a VM value
func reflectValueToVM(reflectVal reflect.Value) vm.Value {
	if !reflectVal.IsValid() {
		return vm.Undefined
	}

	// Check if the value is already a vm.Value
	if reflectVal.Type() == reflect.TypeOf(vm.Value{}) {
		return reflectVal.Interface().(vm.Value)
	}

	switch reflectVal.Kind() {
	case reflect.String:
		return vm.NewString(reflectVal.String())
	case reflect.Float64, reflect.Float32:
		return vm.NumberValue(reflectVal.Float())
	case reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8:
		return vm.NumberValue(float64(reflectVal.Int()))
	case reflect.Uint, reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8:
		return vm.NumberValue(float64(reflectVal.Uint()))
	case reflect.Bool:
		return vm.BooleanValue(reflectVal.Bool())
	case reflect.Map:
		// Convert Go map to VM object
		obj := vm.NewObject(vm.Undefined)
		objPtr := obj.AsPlainObject()
		for _, key := range reflectVal.MapKeys() {
			keyStr := reflectValueToVM(key).ToString()
			valVM := reflectValueToVM(reflectVal.MapIndex(key))
			objPtr.SetOwn(keyStr, valVM)
		}
		return obj
	case reflect.Ptr:
		// Handle struct pointers
		if reflectVal.IsNil() {
			return vm.Null
		}
		if reflectVal.Elem().Kind() == reflect.Struct {
			// Create a VM object to represent the struct instance
			instance := vm.NewObject(vm.Undefined)
			instanceObj := instance.AsPlainObject()

			// Get the struct type
			structType := reflectVal.Elem().Type()

			// Create a temporary ModuleBuilder to use its helper methods
			// This is a bit hacky but works for now
			mb := &ModuleBuilder{vm: nil} // vm not needed for field binding

			// Bind all fields and methods from the Go struct to the VM object
			mb.bindStructMethods(instanceObj, reflectVal, structType)

			return instance
		}
		// For other pointer types, try to dereference
		return reflectValueToVM(reflectVal.Elem())
	case reflect.Slice:
		// Handle slices, particularly []byte -> Uint8Array
		if reflectVal.Type().Elem().Kind() == reflect.Uint8 {
			// Convert []byte to Uint8Array using vm.NewArrayBuffer + vm.NewTypedArray
			goBytes := reflectVal.Bytes()

			// Create ArrayBuffer with the correct size
			arrayBufferValue := vm.NewArrayBuffer(len(goBytes))
			if buffer := arrayBufferValue.AsArrayBuffer(); buffer != nil {
				// Copy the Go bytes into the ArrayBuffer
				copy(buffer.GetData(), goBytes)
				// Create Uint8Array from the ArrayBuffer
				return vm.NewTypedArray(vm.TypedArrayUint8, buffer, 0, 0)
			}
			return vm.Undefined
		}
		// For other slice types, convert to JS array
		arr := vm.NewArray()
		arrayObj := arr.AsArray()
		length := reflectVal.Len()
		for i := 0; i < length; i++ {
			elem := reflectValueToVM(reflectVal.Index(i))
			arrayObj.Set(i, elem)
		}
		return arr
	default:
		return vm.Undefined
	}
}

// ValueConverter methods

func NewValueConverter(vm *vm.VM) *ValueConverter {
	return &ValueConverter{vm: vm}
}

func (vc *ValueConverter) ConvertToVM(goValue interface{}) vm.Value {
	switch v := goValue.(type) {
	case string:
		return vm.NewString(v)
	case int:
		return vm.NumberValue(float64(v))
	case int64:
		return vm.NumberValue(float64(v))
	case float32:
		return vm.NumberValue(float64(v))
	case float64:
		return vm.NumberValue(v)
	case bool:
		return vm.BooleanValue(v)
	case nil:
		return vm.Null
	default:
		// Handle functions using reflection
		if reflect.TypeOf(goValue).Kind() == reflect.Func {
			return vc.wrapGoFunction(goValue)
		}

		// For now, return undefined for unknown types
		return vm.Undefined
	}
}

func (vc *ValueConverter) wrapGoFunction(fn interface{}) vm.Value {
	fnValue := reflect.ValueOf(fn)
	fnType := reflect.TypeOf(fn)

	return vm.NewNativeFunction(fnType.NumIn(), fnType.IsVariadic(), "native_function", func(args []vm.Value) (vm.Value, error) {
		// Convert VM values to Go values for input. Capped at fnType.NumIn()
		// (paserati#278) - see the identical comment in
		// createClassConstructor.
		goArgs := make([]reflect.Value, 0, fnType.NumIn())
		for i := 0; i < len(args) && i < fnType.NumIn(); i++ {
			goArgs = append(goArgs, vc.convertVMValueToReflectValue(args[i], fnType.In(i)))
		}

		// Add missing arguments as zero values if function expects more
		for i := len(args); i < fnType.NumIn(); i++ {
			goArgs = append(goArgs, reflect.Zero(fnType.In(i)))
		}

		// Call the Go function
		results := fnValue.Call(goArgs)

		// Convert result back to VM value
		if len(results) > 0 {
			return vc.convertReflectValueToVM(results[0]), nil
		}

		return vm.Undefined, nil
	})
}

func (vc *ValueConverter) convertVMValueToReflectValue(vmVal vm.Value, targetType reflect.Type) reflect.Value {
	switch targetType.Kind() {
	case reflect.String:
		if vmVal.IsString() {
			return reflect.ValueOf(vmVal.AsString())
		}
		return reflect.ValueOf(vmVal.ToString())
	case reflect.Float64:
		if vmVal.IsNumber() {
			return reflect.ValueOf(vmVal.ToFloat())
		}
		return reflect.ValueOf(0.0)
	case reflect.Float32:
		if vmVal.IsNumber() {
			return reflect.ValueOf(float32(vmVal.ToFloat()))
		}
		return reflect.ValueOf(float32(0.0))
	case reflect.Int, reflect.Int64:
		if vmVal.IsNumber() {
			return reflect.ValueOf(int64(vmVal.ToFloat())).Convert(targetType)
		}
		return reflect.Zero(targetType)
	case reflect.Bool:
		if vmVal.IsBoolean() {
			return reflect.ValueOf(vmVal.AsBoolean())
		}
		return reflect.ValueOf(false)
	default:
		return reflect.Zero(targetType)
	}
}

func (vc *ValueConverter) convertReflectValueToVM(reflectVal reflect.Value) vm.Value {
	if !reflectVal.IsValid() {
		return vm.Undefined
	}

	switch reflectVal.Kind() {
	case reflect.String:
		return vm.NewString(reflectVal.String())
	case reflect.Float64, reflect.Float32:
		return vm.NumberValue(reflectVal.Float())
	case reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8:
		return vm.NumberValue(float64(reflectVal.Int()))
	case reflect.Uint, reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8:
		return vm.NumberValue(float64(reflectVal.Uint()))
	case reflect.Bool:
		return vm.BooleanValue(reflectVal.Bool())
	case reflect.Map:
		// Convert Go map to VM object
		obj := vm.NewObject(vm.Undefined)
		objPtr := obj.AsPlainObject()
		for _, key := range reflectVal.MapKeys() {
			keyStr := vc.convertReflectValueToVM(key).ToString()
			valVM := vc.convertReflectValueToVM(reflectVal.MapIndex(key))
			objPtr.SetOwn(keyStr, valVM)
		}
		return obj
	default:
		return vm.Undefined
	}
}

// TypeGenerator methods

func NewTypeGenerator() *TypeGenerator {
	return &TypeGenerator{
		typeCache: make(map[reflect.Type]types.Type),
	}
}

func (tg *TypeGenerator) GenerateType(goValue interface{}) types.Type {
	if goValue == nil {
		return types.Null
	}

	t := reflect.TypeOf(goValue)

	// Check cache first
	if cached, exists := tg.typeCache[t]; exists {
		return cached
	}

	var result types.Type

	switch t.Kind() {
	case reflect.String:
		result = types.String
	case reflect.Float64, reflect.Float32:
		result = types.Number
	case reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8,
		reflect.Uint, reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8:
		result = types.Number
	case reflect.Bool:
		result = types.Boolean
	case reflect.Func:
		result = tg.generateFunctionType(t)
	case reflect.Map:
		result = mapTypeToObjectType(t, tg.mapGoTypeToTS(t.Elem()))
	default:
		result = types.Any
	}

	tg.typeCache[t] = result
	return result
}

func (tg *TypeGenerator) generateFunctionType(t reflect.Type) types.Type {
	// Build parameter types
	params := make([]types.Type, t.NumIn())
	for i := 0; i < t.NumIn(); i++ {
		params[i] = tg.mapGoTypeToTS(t.In(i))
	}

	// Build return type
	var returnType types.Type = types.Void
	if t.NumOut() > 0 {
		returnType = tg.mapGoTypeToTS(t.Out(0))
	}

	return types.NewSimpleFunction(params, returnType)
}

func (tg *TypeGenerator) mapGoTypeToTS(t reflect.Type) types.Type {
	switch t.Kind() {
	case reflect.String:
		return types.String
	case reflect.Float64, reflect.Float32:
		return types.Number
	case reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8,
		reflect.Uint, reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8:
		return types.Number
	case reflect.Bool:
		return types.Boolean
	case reflect.Map:
		return mapTypeToObjectType(t, goTypeToTSType(t.Elem()))
	default:
		return types.Any
	}
}

// Implementation methods for NativeModuleResolver

func NewNativeModuleResolver() *NativeModuleResolver {
	return &NativeModuleResolver{
		modules:  make(map[string]*NativeModule),
		priority: -100, // High priority to resolve native modules first
	}
}

func (r *NativeModuleResolver) Name() string {
	return "native"
}

func (r *NativeModuleResolver) CanResolve(specifier string) bool {
	_, exists := r.modules[specifier]
	return exists
}

func (r *NativeModuleResolver) Priority() int {
	return r.priority
}

func (r *NativeModuleResolver) RegisterModule(name string, module *NativeModule) {
	r.modules[name] = module
}

// NativeModuleSource implements io.ReadCloser for native modules
type NativeModuleSource struct {
	module         *NativeModule
	content        []byte
	pos            int
	isNativeModule bool // Flag to indicate this is a native module
}

func (nms *NativeModuleSource) Read(p []byte) (int, error) {
	if nms.content == nil {
		// Generate synthetic TypeScript source for the module
		nms.content = nms.generateSyntheticSource()
	}

	if nms.pos >= len(nms.content) {
		return 0, io.EOF
	}

	n := copy(p, nms.content[nms.pos:])
	nms.pos += n
	return n, nil
}

func (nms *NativeModuleSource) Close() error {
	return nil
}

// IsNativeModule implements the NativeModuleSource interface
func (nms *NativeModuleSource) IsNativeModule() bool {
	return nms.isNativeModule
}

// GetNativeModule implements the NativeModuleSource interface
func (nms *NativeModuleSource) GetNativeModule() interface{} {
	return nms.module
}

func (nms *NativeModuleSource) generateSyntheticSource() []byte {
	mod := nms.module
	exports := mod.GetExports()

	var sb strings.Builder
	fmt.Fprintf(&sb, "// Auto-generated native module: %s\n", mod.name)

	if len(exports) == 0 {
		sb.WriteString("export {}\n")
		return []byte(sb.String())
	}

	names := make([]string, 0, len(exports))
	for name := range exports {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		typeStr := exportTypeString(exports[name])
		switch name {
		case "default":
			fmt.Fprintf(&sb, "declare const __default: %s;\n", typeStr)
			sb.WriteString("export default __default;\n")
		default:
			if isValidExportIdentifier(name) {
				fmt.Fprintf(&sb, "export declare const %s: %s;\n", name, typeStr)
			}
		}
	}

	return []byte(sb.String())
}

func exportTypeString(typ types.Type) string {
	if typ == nil {
		return "any"
	}
	if s := typ.String(); s != "" {
		return s
	}
	return "any"
}

func isValidExportIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if r != '_' && r != '$' && !unicode.IsLetter(r) {
				return false
			}
		} else if r != '_' && r != '$' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return lexer.LookupIdent(name) == lexer.IDENT
}

func (r *NativeModuleResolver) Resolve(specifier string, fromPath string) (*modules.ResolvedModule, error) {
	module, exists := r.modules[specifier]
	if !exists {
		return nil, fmt.Errorf("native module '%s' not found", specifier)
	}

	return &modules.ResolvedModule{
		Specifier:    specifier,
		ResolvedPath: "native://" + module.name,
		Source:       &NativeModuleSource{module: module, isNativeModule: true},
		Resolver:     "native",
	}, nil
}

// DeclareModule adds a native module declaration to the Paserati instance
func (p *Paserati) DeclareModule(name string, builder func(m *ModuleBuilder)) *NativeModule {
	module := &NativeModule{
		name:    name,
		builder: builder,
	}

	// Add to native resolver if not exists
	if p.nativeResolver == nil {
		p.nativeResolver = NewNativeModuleResolver()
		p.moduleLoader.AddResolver(p.nativeResolver)
	}

	p.nativeResolver.RegisterModule(name, module)
	return module
}

// DeclareModuleAlias registers alias as another specifier for an existing
// native module (e.g. "node:fs" → "fs"). Both specifiers resolve to the same
// NativeModule and, once loaded, the same ModuleRecord.
func (p *Paserati) DeclareModuleAlias(alias, canonical string) error {
	if alias == "" || canonical == "" {
		return fmt.Errorf("DeclareModuleAlias: alias and name must be non-empty")
	}
	if p.nativeResolver == nil {
		return fmt.Errorf("DeclareModuleAlias: no native module %q (call DeclareModule first)", canonical)
	}
	return p.nativeResolver.RegisterAlias(alias, canonical)
}

func (r *NativeModuleResolver) RegisterAlias(alias, canonical string) error {
	module, ok := r.modules[canonical]
	if !ok {
		return fmt.Errorf("native module %q is not declared", canonical)
	}
	if existing, exists := r.modules[alias]; exists && existing != module {
		return fmt.Errorf("alias %q is already registered to a different module", alias)
	}
	r.modules[alias] = module
	return nil
}

// GetName implements the NativeModuleInterface
func (nm *NativeModule) GetName() string {
	return nm.name
}

// InitializeExports implements the NativeModuleInterface
func (nm *NativeModule) InitializeExports(vmInstance *vm.VM) map[string]vm.Value {
	return nm.initializeNativeModule(vmInstance)
}

// initializeNativeModule initializes a native module and returns its runtime values
func (nm *NativeModule) initializeNativeModule(vmInstance *vm.VM) map[string]vm.Value {
	nm.mutex.Do(func() {
		// Create the module builder with direct type/value synthesis
		builder := &ModuleBuilder{
			exports: make(map[string]types.Type),
			values:  make(map[string]vm.Value),
			vm:      vmInstance,
		}

		// Call the user's builder function
		nm.builder(builder)
		builder.applyDefaultExport()

		// Store the results
		nm.exports = builder.exports // Type information
		nm.values = builder.values   // Runtime values
		nm.initialized = true
	})

	return nm.values // Return runtime values for VM
}

// GetExports returns the type information for this native module
func (nm *NativeModule) GetExports() map[string]types.Type {
	if !nm.initialized {
		return make(map[string]types.Type)
	}
	return nm.exports
}

// GetTypeExports implements the NativeModuleInterface
func (nm *NativeModule) GetTypeExports() map[string]types.Type {
	return nm.GetExports()
}

// IsNativeModuleSource checks if a source is a native module source
func IsNativeModuleSource(source interface{}) (*NativeModule, bool) {
	if nms, ok := source.(*NativeModuleSource); ok && nms.isNativeModule {
		return nms.module, true
	}
	return nil, false
}

// HandleNativeModule processes a native module and populates the module record
func HandleNativeModule(nativeModule *NativeModule, record *modules.ModuleRecord, vmInstance *vm.VM) error {
	// Initialize the native module to get its exports
	exports := nativeModule.initializeNativeModule(vmInstance)

	// Populate the module record with native module exports
	record.ExportValues = exports
	record.State = modules.ModuleCompiled // Mark as compiled since no compilation needed

	// Create a simple AST to satisfy the module system requirements
	// This is just a placeholder since we don't need real parsing for native modules
	record.AST = &parser.Program{
		Statements: []parser.Statement{}, // Empty program is fine
	}

	// For type information, we can populate the Exports field if needed
	if len(nativeModule.exports) > 0 {
		record.Exports = nativeModule.exports
	}

	return nil
}

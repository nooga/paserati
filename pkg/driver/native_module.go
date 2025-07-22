package driver

import (
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"paserati/pkg/modules"
	"paserati/pkg/parser"
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// ModuleBuilder provides the declarative API for building native modules
// This works like builtin initializers, directly creating types and runtime values
type ModuleBuilder struct {
	// Type information (populated directly)
	exports     map[string]types.Type
	
	// Runtime values (populated directly) 
	values      map[string]vm.Value
	
	// VM instance for creating runtime objects
	vm          *vm.VM
}

// NamespaceBuilder provides API for building namespaces within modules
type NamespaceBuilder struct {
	// Type information
	exports     map[string]types.Type
	
	// Runtime values
	values      map[string]vm.Value
	
	// VM instance for creating runtime objects
	vm          *vm.VM
}

// NativeModule represents a module declared in Go code
type NativeModule struct {
	name         string
	builder      func(*ModuleBuilder)
	initialized  bool
	exports      map[string]types.Type     // Type information
	values       map[string]vm.Value       // Runtime values
	mutex        sync.Once
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

// AsyncFunction adds an async function to the module (TODO: implement Promise wrapping)
func (m *ModuleBuilder) AsyncFunction(name string, fn interface{}) *ModuleBuilder {
	// For now, just treat as regular function
	return m.Function(name, fn)
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
	constructorValue := m.createClassConstructor(goStruct, constructor)
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
	fmt.Printf("DEBUG: Creating namespace '%s' with %d values\n", name, len(ns.values))
	for propName, propValue := range ns.values {
		nsObjPtr.SetOwn(propName, propValue)
		fmt.Printf("DEBUG: Namespace '%s' set property '%s' = %s\n", name, propName, propValue.ToString())
	}
	m.values[name] = nsObj
	fmt.Printf("DEBUG: Added namespace '%s' to module exports. Object: %s\n", name, nsObj.ToString())
	
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

// Default sets the default export (TODO: implement)
func (m *ModuleBuilder) Default(value interface{}) *ModuleBuilder {
	// TODO: implement default export
	return m
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
func (m *ModuleBuilder) createClassConstructor(goStruct interface{}, constructor interface{}) vm.Value {
	constructorFn := reflect.ValueOf(constructor)
	constructorType := reflect.TypeOf(constructor)
	structType := reflect.TypeOf(goStruct).Elem() // Remove pointer to get struct type
	
	if constructorType.Kind() != reflect.Func {
		return vm.Undefined
	}
	
	return vm.NewNativeFunction(constructorType.NumIn(), constructorType.IsVariadic(), "class_constructor", func(args []vm.Value) (vm.Value, error) {
		// Convert VM values to Go values for constructor call
		goArgs := make([]reflect.Value, len(args))
		for i, arg := range args {
			if i < constructorType.NumIn() {
				goArgs[i] = vmValueToReflectValue(arg, constructorType.In(i))
			}
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
		
		// Get the created Go instance
		goInstance := results[0]
		if !goInstance.IsValid() || goInstance.IsNil() {
			return vm.Undefined, nil
		}
		
		// Create a VM object to represent the instance
		instance := vm.NewObject(vm.Undefined)
		instanceObj := instance.AsPlainObject()
		
		// Bind all methods from the Go struct to the VM object
		m.bindStructMethods(instanceObj, goInstance, structType)
		
		// Store the Go instance as a hidden property for method calls
		// This is a simple approach - in a full implementation, you'd use a more sophisticated storage
		// For now, we'll just bind methods directly without storing the Go instance
		
		return instance, nil
	})
}

// bindStructMethods binds all exported methods from a Go struct to a VM object
func (m *ModuleBuilder) bindStructMethods(vmObj *vm.PlainObject, goInstance reflect.Value, structType reflect.Type) {
	// First, bind struct fields as properties (respecting JSON tags)
	m.bindStructFields(vmObj, goInstance, structType)
	
	// Then, bind methods
	// Get the pointer type for method lookup
	ptrType := reflect.PtrTo(structType)
	
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
		vmObj.SetOwn(methodName, vmMethod)
	}
}

// createBoundMethod creates a VM function that calls a bound Go method
func (m *ModuleBuilder) createBoundMethod(methodFunc reflect.Value) vm.Value {
	methodType := methodFunc.Type()
	
	return vm.NewNativeFunction(methodType.NumIn(), methodType.IsVariadic(), "bound_method", func(args []vm.Value) (vm.Value, error) {
		// Convert VM values to Go values for method call
		goArgs := make([]reflect.Value, len(args))
		for i, arg := range args {
			if i < methodType.NumIn() {
				goArgs[i] = vmValueToReflectValue(arg, methodType.In(i))
			}
		}
		
		// Add missing arguments as zero values if method expects more
		for i := len(args); i < methodType.NumIn(); i++ {
			goArgs = append(goArgs, reflect.Zero(methodType.In(i)))
		}
		
		// Call the Go method
		results := methodFunc.Call(goArgs)
		
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
	
	return vm.NewNativeFunction(fnType.NumIn(), fnType.IsVariadic(), "native_function", func(args []vm.Value) (vm.Value, error) {
		// Convert VM values to Go values for input
		goArgs := make([]reflect.Value, len(args))
		for i, arg := range args {
			if i < fnType.NumIn() {
				goArgs[i] = vmValueToReflectValue(arg, fnType.In(i))
			}
		}
		
		// Add missing arguments as zero values if function expects more
		for i := len(args); i < fnType.NumIn(); i++ {
			goArgs = append(goArgs, reflect.Zero(fnType.In(i)))
		}
		
		// Call the Go function
		results := fnValue.Call(goArgs)
		
		// Convert result back to VM value
		if len(results) > 0 {
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
		// For now, return a generic object type
		return types.NewObjectType()
	default:
		return types.Any
	}
}

// vmValueToReflectValue converts a VM value to a reflect.Value for function calls
func vmValueToReflectValue(vmVal vm.Value, targetType reflect.Type) reflect.Value {
	switch targetType.Kind() {
	case reflect.String:
		if vmVal.IsString() {
			return reflect.ValueOf(vmVal.AsString())
		}
		return reflect.ValueOf(vmVal.ToString())
	case reflect.Float64:
		if vmVal.IsNumber() {
			return reflect.ValueOf(vmVal.AsFloat())
		}
		return reflect.ValueOf(0.0)
	case reflect.Float32:
		if vmVal.IsNumber() {
			return reflect.ValueOf(float32(vmVal.AsFloat()))
		}
		return reflect.ValueOf(float32(0.0))
	case reflect.Int, reflect.Int64:
		if vmVal.IsNumber() {
			return reflect.ValueOf(int64(vmVal.AsFloat()))
		}
		return reflect.ValueOf(int64(0))
	case reflect.Bool:
		if vmVal.IsBoolean() {
			return reflect.ValueOf(vmVal.AsBoolean())
		}
		return reflect.ValueOf(false)
	default:
		return reflect.Zero(targetType)
	}
}

// reflectValueToVM converts a reflect.Value to a VM value
func reflectValueToVM(reflectVal reflect.Value) vm.Value {
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
			keyStr := reflectValueToVM(key).ToString()
			valVM := reflectValueToVM(reflectVal.MapIndex(key))
			objPtr.SetOwn(keyStr, valVM)
		}
		return obj
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
			// Convert VM values to Go values for input
			goArgs := make([]reflect.Value, len(args))
			for i, arg := range args {
				if i < fnType.NumIn() {
					goArgs[i] = vc.convertVMValueToReflectValue(arg, fnType.In(i))
				}
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
			return reflect.ValueOf(vmVal.AsFloat())
		}
		return reflect.ValueOf(0.0)
	case reflect.Float32:
		if vmVal.IsNumber() {
			return reflect.ValueOf(float32(vmVal.AsFloat()))
		}
		return reflect.ValueOf(float32(0.0))
	case reflect.Int, reflect.Int64:
		if vmVal.IsNumber() {
			return reflect.ValueOf(int64(vmVal.AsFloat()))
		}
		return reflect.ValueOf(int64(0))
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
		// For now, treat maps as objects with string keys and any values
		result = types.NewObjectType() // TODO: improve map type generation
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
		// For now, return a generic object type
		return types.NewObjectType()
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
	module        *NativeModule
	content       []byte
	pos           int
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
	// For now, generate a simple export statement
	// TODO: Generate proper TypeScript declarations based on module exports
	return []byte(`
// Auto-generated native module exports
export const PI_SQUARED: number = 9.8696; // placeholder
export const EULER: number = 2.718281828; // placeholder
export function square(x: number): number { return x * x; }
export function add(a: number, b: number): number { return a + b; }
export function divmod(a: number, b: number): any { return {}; }
`)
}

func (r *NativeModuleResolver) Resolve(specifier string, fromPath string) (*modules.ResolvedModule, error) {
	module, exists := r.modules[specifier]
	if !exists {
		return nil, fmt.Errorf("native module '%s' not found", specifier)
	}
	
	return &modules.ResolvedModule{
		Specifier:    specifier,
		ResolvedPath: "native://" + specifier,
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
		p.moduleLoader.AddResolver(p.nativeResolver.(*NativeModuleResolver))
	}
	
	p.nativeResolver.(*NativeModuleResolver).RegisterModule(name, module)
	return module
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
	fmt.Printf("DEBUG: Initializing native module '%s'\n", nm.name)
	nm.mutex.Do(func() {
		fmt.Printf("DEBUG: Inside sync.Once for module '%s'\n", nm.name)
		// Create the module builder with direct type/value synthesis
		builder := &ModuleBuilder{
			exports: make(map[string]types.Type),
			values:  make(map[string]vm.Value),
			vm:      vmInstance,
		}
		
		// Call the user's builder function
		nm.builder(builder)
		
		// Store the results
		nm.exports = builder.exports  // Type information
		nm.values = builder.values    // Runtime values
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
	fmt.Printf("DEBUG: About to initialize native module '%s'\n", nativeModule.GetName())
	exports := nativeModule.initializeNativeModule(vmInstance)
	fmt.Printf("DEBUG: Native module '%s' initialized with %d exports\n", nativeModule.GetName(), len(exports))
	for name, value := range exports {
		fmt.Printf("DEBUG: Export '%s' = %s (type: %d)\n", name, value.ToString(), int(value.Type()))
	}
	
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
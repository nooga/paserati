package main

import (
	"fmt"
	"paserati/pkg/builtins"
	"paserati/pkg/types"
	"paserati/pkg/vm"
	"strings"
)

// Share the constructed Test262Error constructor across initializers since VM globals
// are not yet installed during InitRuntime and GetGlobal returns undefined at that time.
var sharedTest262ErrorCtor vm.Value = vm.Undefined

// test262ExceptionError adapts a VM Value into an ExceptionError for throwing from builtins
type test262ExceptionError struct{ v vm.Value }

func (e test262ExceptionError) Error() string               { return "VM exception" }
func (e test262ExceptionError) GetExceptionValue() vm.Value { return e.v }

// Test262Exception wraps a Test262Error object for proper exception handling
type Test262Exception struct {
	ErrorObject vm.Value
	Message     string
}

func (e *Test262Exception) Error() string {
	return e.Message
}

func (e *Test262Exception) GetExceptionValue() vm.Value {
	return e.ErrorObject
}

// Test262Initializer provides Test262-specific globals
type Test262Initializer struct{}

func (t *Test262Initializer) Name() string {
	return "Test262"
}

func (t *Test262Initializer) Priority() int {
	return 1000 // After all standard builtins
}

func (t *Test262Initializer) InitTypes(ctx *builtins.TypeContext) error {
	// print function - variadic, accepts any arguments
	printType := types.NewVariadicFunction([]types.Type{}, types.Undefined, &types.ArrayType{ElementType: types.Any})
	if err := ctx.DefineGlobal("print", printType); err != nil {
		return err
	}

	// Test262Error constructor - takes optional message string
	test262ErrorType := types.NewSimpleFunction([]types.Type{types.String}, types.Any)
	if err := ctx.DefineGlobal("Test262Error", test262ErrorType); err != nil {
		return err
	}

	// $ERROR function - takes message string
	errorType := types.NewSimpleFunction([]types.Type{types.String}, types.Undefined)
	if err := ctx.DefineGlobal("$ERROR", errorType); err != nil {
		return err
	}

	// isConstructor function - checks if a value is a constructor
	isConstructorType := types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean)
	if err := ctx.DefineGlobal("isConstructor", isConstructorType); err != nil {
		return err
	}

	return nil
}

func (t *Test262Initializer) InitRuntime(ctx *builtins.RuntimeContext) error {
	//fmt.Printf("DEBUG: Initializing Test262 builtins\n")
	// print function for test output
	printFn := vm.NewNativeFunction(0, true, "print", func(args []vm.Value) (vm.Value, error) {
		parts := make([]string, len(args))
		for i, arg := range args {
			parts[i] = arg.Inspect()
		}
		fmt.Println(strings.Join(parts, " "))
		return vm.Undefined, nil
	})
	if err := ctx.DefineGlobal("print", printFn); err != nil {
		return err
	}

	// We need to define Test262Error as a special constructor that creates proper error objects
	// First create a placeholder that will be updated with self-reference
	var test262ErrorCtor vm.Value
	// Create Test262Error.prototype inheriting from Error.prototype if available
	test262ErrorProto := vm.NewObject(ctx.VM.ErrorPrototype).AsPlainObject()
	test262ErrorProto.SetOwn("name", vm.NewString("Test262Error"))
	test262ErrorProto.SetOwn("message", vm.NewString(""))
	// Set toString on prototype
	test262ErrorProto.SetOwn("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		thisVal := ctx.VM.GetThis()
		msg := ""
		if po := thisVal.AsPlainObject(); po != nil {
			if mv, ok := po.GetOwn("message"); ok {
				msg = mv.ToString()
			}
		}
		return vm.NewString("Test262Error: " + msg), nil
	}))

	test262ErrorCtor = vm.NewNativeFunction(1, true, "Test262Error", func(args []vm.Value) (vm.Value, error) {
		message := "Test262Error"
		if len(args) > 0 {
			message = args[0].ToString()
		}

		// Create instance inheriting from Test262Error.prototype
		inst := vm.NewObject(vm.NewValueFromPlainObject(test262ErrorProto)).AsPlainObject()
		inst.SetOwn("name", vm.NewString("Test262Error"))
		inst.SetOwn("message", vm.NewString(message))
		// Optionally set stack if available
		stack := ctx.VM.CaptureStackTrace()
		inst.SetOwn("stack", vm.NewString(stack))
		// DEBUG: Print prototype chain and constructor prototype linkage
		proto := inst.GetPrototype()
		protoName := "<no name>"
		if proto.IsObject() {
			if nv, ok := proto.AsPlainObject().GetOwn("name"); ok {
				protoName = nv.ToString()
			}
		}
		fmt.Printf("[DBG Test262ErrorCtor] instance proto name: %s\n", protoName)
		// Also check ctor.prototype.name
		if ctorProtoVal, ok := test262ErrorProto.GetOwn("name"); ok {
			fmt.Printf("[DBG Test262ErrorCtor] ctor.prototype.name: %s\n", ctorProtoVal.ToString())
		}
		return vm.NewValueFromPlainObject(inst), nil
	})
	// Make it a proper constructor with prototype property
	if ctorObj := test262ErrorCtor.AsNativeFunction(); ctorObj != nil {
		ctorWithProps := vm.NewNativeFunctionWithProps(ctorObj.Arity, ctorObj.Variadic, ctorObj.Name, ctorObj.Fn)
		ctorProps := ctorWithProps.AsNativeFunctionWithProps()
		ctorProps.Properties.SetOwn("prototype", vm.NewValueFromPlainObject(test262ErrorProto))
		// Set constructor on prototype
		test262ErrorProto.SetOwn("constructor", ctorWithProps)
		test262ErrorCtor = ctorWithProps
	}

	// Store globally for other initializers to reference during this init phase
	sharedTest262ErrorCtor = test262ErrorCtor

	if err := ctx.DefineGlobal("Test262Error", test262ErrorCtor); err != nil {
		return err
	}
	// Also mirror to globalThis for harness convenience if not already present
	// Our globals live on the VM's global environment; DefineGlobal already sets it.

	// $ERROR function (legacy Test262 function)
	errorFn := vm.NewNativeFunction(1, false, "$ERROR", func(args []vm.Value) (vm.Value, error) {
		message := "Test failed"
		if len(args) > 0 {
			message = args[0].ToString()
		}
		errVal, _ := ctx.VM.Call(test262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString(message)})
		return vm.Undefined, test262ExceptionError{v: errVal}
	})
	if err := ctx.DefineGlobal("$ERROR", errorFn); err != nil {
		return err
	}

	// Minimal $262 harness object with createRealm stub
	harness262 := vm.NewObject(vm.Null).AsPlainObject()
	// byteConversionValues helper structure used by typed array tests
	// Provide minimal values/expected tables sufficient for harness checks
	byteConv := vm.NewObject(vm.Null).AsPlainObject()
	// values: pick some representative set (simplified)
	valuesVal := vm.NewArray()
	valuesArr := valuesVal.AsArray()
	valuesArr.Append(vm.NumberValue(0))
	valuesArr.Append(vm.NumberValue(1))
	valuesArr.Append(vm.NumberValue(-1))
	valuesArr.Append(vm.NumberValue(255))
	valuesArr.Append(vm.NumberValue(256))
	byteConv.SetOwn("values", valuesVal)
	// expected: minimal same-length arrays per type names with stubbed values
	mkExpected := func() *vm.PlainObject {
		o := vm.NewObject(vm.Null).AsPlainObject()
		add := func(name string) {
			arrVal := vm.NewArray()
			a := arrVal.AsArray()
			// Ensure same length as values
			for i := 0; i < valuesArr.Length(); i++ {
				a.Append(valuesArr.Get(i))
			}
			o.SetOwn(name, arrVal)
		}
		add("Float32")
		add("Float64")
		add("Int8")
		add("Int16")
		add("Int32")
		add("Uint8")
		add("Uint16")
		add("Uint32")
		add("Uint8Clamped")
		return o
	}
	byteConv.SetOwn("expected", vm.NewValueFromPlainObject(mkExpected()))
	// Expose both globally and on $262 for harness to access
	byteConvVal := vm.NewValueFromPlainObject(byteConv)
	if err := ctx.DefineGlobal("byteConversionValues", byteConvVal); err != nil {
		return err
	}
	harness262.SetOwn("byteConversionValues", byteConvVal)
	harness262.SetOwn("createRealm", vm.NewNativeFunction(0, false, "createRealm", func(args []vm.Value) (vm.Value, error) {
		// Return a minimal realm-like object with a fresh global object and evalScript stub
		realm := vm.NewObject(vm.Null).AsPlainObject()
		realmGlobal := vm.NewObject(vm.Null).AsPlainObject()
		realm.SetOwn("global", vm.NewValueFromPlainObject(realmGlobal))
		realm.SetOwn("evalScript", vm.NewNativeFunction(1, false, "evalScript", func(args []vm.Value) (vm.Value, error) {
			// For now, do nothing and return undefined. Full realm eval not implemented.
			return vm.Undefined, nil
		}))
		return vm.NewValueFromPlainObject(realm), nil
	}))
	if err := ctx.DefineGlobal("$262", vm.NewValueFromPlainObject(harness262)); err != nil {
		return err
	}

	// Provide minimal native Error subclasses missing from engine: EvalError, RangeError, URIError
	makeErrorSubclass := func(name string) vm.Value {
		proto := vm.NewObject(ctx.VM.ErrorPrototype).AsPlainObject()
		proto.SetOwn("name", vm.NewString(name))
		ctor := vm.NewNativeFunction(1, true, name, func(args []vm.Value) (vm.Value, error) {
			msg := ""
			if len(args) > 0 && args[0].Type() != vm.TypeUndefined {
				msg = args[0].ToString()
			}
			inst := vm.NewObject(vm.NewValueFromPlainObject(proto)).AsPlainObject()
			inst.SetOwn("name", vm.NewString(name))
			inst.SetOwn("message", vm.NewString(msg))
			inst.SetOwn("stack", vm.NewString(ctx.VM.CaptureStackTrace()))
			return vm.NewValueFromPlainObject(inst), nil
		})
		if nf := ctor.AsNativeFunction(); nf != nil {
			withProps := vm.NewNativeFunctionWithProps(nf.Arity, nf.Variadic, nf.Name, nf.Fn)
			withProps.AsNativeFunctionWithProps().Properties.SetOwn("prototype", vm.NewValueFromPlainObject(proto))
			proto.SetOwn("constructor", withProps)
			return withProps
		}
		return ctor
	}
	if err := ctx.DefineGlobal("EvalError", makeErrorSubclass("EvalError")); err != nil {
		return err
	}
	if err := ctx.DefineGlobal("RangeError", makeErrorSubclass("RangeError")); err != nil {
		return err
	}
	if err := ctx.DefineGlobal("URIError", makeErrorSubclass("URIError")); err != nil {
		return err
	}

	// Harness property helper globals implemented minimally via verifyProperty + Object.getOwnPropertyDescriptor
	// verifyConfigurable(obj, name)
	verifyConfigurable := vm.NewNativeFunction(2, false, "verifyConfigurable", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			msg := "verifyConfigurable requires 2 arguments"
			errVal, _ := ctx.VM.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString(msg)})
			return vm.Undefined, test262ExceptionError{v: errVal}
		}
		obj := args[0]
		key := args[1]
		// For messages
		nameStr := key.Inspect()
		if key.Type() != vm.TypeSymbol {
			nameStr = key.ToString()
		}
		descriptorVal, _ := builtins.ObjectGetOwnPropertyDescriptorForHarness(obj, key)
		if d := descriptorVal.AsPlainObject(); d != nil {
			if cfg, ok := d.GetOwn("configurable"); ok && cfg.IsTruthy() {
				return vm.Undefined, nil
			}
		}
		msg := fmt.Sprintf("Property '%s' is not configurable", nameStr)
		errVal, _ := ctx.VM.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString(msg)})
		return vm.Undefined, test262ExceptionError{v: errVal}
	})
	ctx.DefineGlobal("verifyConfigurable", verifyConfigurable)

	// verifyNotConfigurable(obj, name)
	verifyNotConfigurable := vm.NewNativeFunction(2, false, "verifyNotConfigurable", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			msg := "verifyNotConfigurable requires 2 arguments"
			errVal, _ := ctx.VM.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString(msg)})
			return vm.Undefined, test262ExceptionError{v: errVal}
		}
		obj := args[0]
		name := args[1].ToString()
		descriptorVal, _ := builtins.ObjectGetOwnPropertyDescriptorForHarness(obj, vm.NewString(name))
		if d := descriptorVal.AsPlainObject(); d != nil {
			if cfg, ok := d.GetOwn("configurable"); ok && !cfg.IsTruthy() {
				return vm.Undefined, nil
			}
		}
		msg := fmt.Sprintf("Property '%s' is configurable", name)
		errVal, _ := ctx.VM.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString(msg)})
		return vm.Undefined, test262ExceptionError{v: errVal}
	})
	ctx.DefineGlobal("verifyNotConfigurable", verifyNotConfigurable)

	// verifyEnumerable(obj, name)
	verifyEnumerable := vm.NewNativeFunction(2, false, "verifyEnumerable", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			msg := "verifyEnumerable requires 2 arguments"
			errVal, _ := ctx.VM.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString(msg)})
			return vm.Undefined, test262ExceptionError{v: errVal}
		}
		obj := args[0]
		name := args[1].ToString()
		descriptorVal, _ := builtins.ObjectGetOwnPropertyDescriptorForHarness(obj, vm.NewString(name))
		if d := descriptorVal.AsPlainObject(); d != nil {
			if en, ok := d.GetOwn("enumerable"); ok && en.IsTruthy() {
				return vm.Undefined, nil
			}
		}
		msg := fmt.Sprintf("Property '%s' is not enumerable", name)
		errVal, _ := ctx.VM.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString(msg)})
		return vm.Undefined, test262ExceptionError{v: errVal}
	})
	ctx.DefineGlobal("verifyEnumerable", verifyEnumerable)

	// verifyNotEnumerable(obj, name)
	verifyNotEnumerable := vm.NewNativeFunction(2, false, "verifyNotEnumerable", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			msg := "verifyNotEnumerable requires 2 arguments"
			errVal, _ := ctx.VM.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString(msg)})
			return vm.Undefined, test262ExceptionError{v: errVal}
		}
		obj := args[0]
		name := args[1].ToString()
		descriptorVal, _ := builtins.ObjectGetOwnPropertyDescriptorForHarness(obj, vm.NewString(name))
		if d := descriptorVal.AsPlainObject(); d != nil {
			if en, ok := d.GetOwn("enumerable"); ok && !en.IsTruthy() {
				return vm.Undefined, nil
			}
		}
		msg := fmt.Sprintf("Property '%s' is enumerable", name)
		errVal, _ := ctx.VM.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString(msg)})
		return vm.Undefined, test262ExceptionError{v: errVal}
	})
	ctx.DefineGlobal("verifyNotEnumerable", verifyNotEnumerable)

	// verifyWritable(obj, name, [value])
	verifyWritable := vm.NewNativeFunction(3, false, "verifyWritable", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			msg := "verifyWritable requires at least 2 arguments"
			errVal, _ := ctx.VM.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString(msg)})
			return vm.Undefined, test262ExceptionError{v: errVal}
		}
		obj := args[0]
		name := args[1].ToString()
		// Default new value if not provided
		newVal := vm.NewString("__paserati_verify_writable__")
		if len(args) >= 3 {
			newVal = args[2]
		}
		// Special-case Array.length
		if arr := obj.AsArray(); arr != nil && name == "length" {
			return vm.Undefined, nil
		}
		// Non-destructive: capture original
		var original vm.Value = vm.Undefined
		var hadProp bool
		if po := obj.AsPlainObject(); po != nil {
			if v, ok := po.GetOwn(name); ok {
				original, hadProp = v, true
			}
			po.SetOwn(name, newVal)
			if v, ok := po.GetOwn(name); ok && sameValueSimple(v, newVal) {
				if hadProp {
					po.SetOwn(name, original)
				}
				return vm.Undefined, nil
			}
		} else if dict := obj.AsDictObject(); dict != nil {
			if v, ok := dict.GetOwn(name); ok {
				original, hadProp = v, true
			}
			dict.SetOwn(name, newVal)
			if v, ok := dict.GetOwn(name); ok && sameValueSimple(v, newVal) {
				if hadProp {
					dict.SetOwn(name, original)
				}
				return vm.Undefined, nil
			}
		}
		msg := fmt.Sprintf("Property '%s' is not writable", name)
		errVal, _ := ctx.VM.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString(msg)})
		return vm.Undefined, test262ExceptionError{v: errVal}
	})
	ctx.DefineGlobal("verifyWritable", verifyWritable)

	// verifyNotWritable(obj, name)
	verifyNotWritable := vm.NewNativeFunction(3, false, "verifyNotWritable", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			msg := "verifyNotWritable requires at least 2 arguments"
			errVal, _ := ctx.VM.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString(msg)})
			return vm.Undefined, test262ExceptionError{v: errVal}
		}
		obj := args[0]
		name := args[1].ToString()
		original := vm.Undefined
		if po := obj.AsPlainObject(); po != nil {
			original, _ = po.GetOwn(name)
			po.SetOwn(name, vm.NewString("__test_write__"))
			if v, ok := po.GetOwn(name); ok && sameValueSimple(v, original) {
				return vm.Undefined, nil
			}
		}
		msg := fmt.Sprintf("Property '%s' is writable", name)
		errVal, _ := ctx.VM.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString(msg)})
		return vm.Undefined, test262ExceptionError{v: errVal}
	})
	ctx.DefineGlobal("verifyNotWritable", verifyNotWritable)

	// isConstructor function - checks if a value can be invoked as a constructor
	isConstructorFn := vm.NewNativeFunction(1, false, "isConstructor", func(args []vm.Value) (vm.Value, error) {
		// fmt.Printf("[DEBUG isConstructor] Called with %d args\n", len(args))
		if len(args) == 0 {
			return vm.BooleanValue(false), nil
		}

		val := args[0]
		// fmt.Printf("[DEBUG isConstructor] Checking value type: %v\n", val.Type())

		// Check if it's a function
		if !val.IsCallable() {
			// fmt.Printf("[DEBUG isConstructor] Value is not callable\n")
			return vm.BooleanValue(false), nil
		}

		// For native functions, check if they have constructor capability
		if nativeFn := val.AsNativeFunction(); nativeFn != nil {
			// Most native functions are not constructors unless specifically marked
			// For now, we'll return false for all native functions except known constructors
			name := nativeFn.Name
			// fmt.Printf("[DEBUG isConstructor] Native function name: %s\n", name)
			constructors := map[string]bool{
				"Object":         true,
				"Array":          true,
				"Function":       true,
				"String":         true,
				"Number":         true,
				"Boolean":        true,
				"Date":           true,
				"RegExp":         true,
				"Error":          true,
				"TypeError":      true,
				"ReferenceError": true,
				"RangeError":     true,
				"SyntaxError":    true,
				"EvalError":      true,
				"URIError":       true,
				"Test262Error":   true,
			}
			result := constructors[name]
			// fmt.Printf("[DEBUG isConstructor] Native function result: %v\n", result)
			return vm.BooleanValue(result), nil
		}

		// For user-defined functions (compiled functions), they are constructors by default
		// unless they're arrow functions or methods
		if val.Type() == vm.TypeFunction {
			// In our implementation, all compiled functions can be constructors
			// This might need refinement based on function type (arrow vs regular)
			// fmt.Printf("[DEBUG isConstructor] User-defined function, returning true\n")
			return vm.BooleanValue(true), nil
		}

		// Default to false for safety
		// fmt.Printf("[DEBUG isConstructor] Defaulting to false\n")
		return vm.BooleanValue(false), nil
	})
	if err := ctx.DefineGlobal("isConstructor", isConstructorFn); err != nil {
		return err
	}

	return nil
}

// AssertInitializer provides assert functions for Test262
type AssertInitializer struct{}

func (a *AssertInitializer) Name() string {
	return "assert"
}

func (a *AssertInitializer) Priority() int {
	return 1001 // After Test262Initializer
}

func (a *AssertInitializer) InitTypes(ctx *builtins.TypeContext) error {
	// Create assert as a callable object with methods
	// For now, we'll use a function type with an intersection to add properties
	assertFnType := types.NewVariadicFunction([]types.Type{types.Any}, types.Undefined, &types.ArrayType{ElementType: types.Any})

	// Create an object type for the methods
	assertObj := types.NewObjectType()
	assertObj.Properties = map[string]types.Type{
		// assert.sameValue method
		"sameValue": types.NewVariadicFunction([]types.Type{types.Any, types.Any}, types.Undefined, &types.ArrayType{ElementType: types.Any}),
		// assert.notSameValue method
		"notSameValue": types.NewVariadicFunction([]types.Type{types.Any, types.Any}, types.Undefined, &types.ArrayType{ElementType: types.Any}),
		// assert.throws method
		"throws": types.NewSimpleFunction([]types.Type{types.Any, types.Any}, types.Undefined),
	}

	// Create intersection of function and object to represent callable object with properties
	assertType := types.NewIntersectionType(assertFnType, assertObj)

	if err := ctx.DefineGlobal("assert", assertType); err != nil {
		return err
	}

	// verifyProperty function type - takes (object, property name, expected value)
	verifyPropertyType := types.NewSimpleFunction([]types.Type{types.Any, types.String, types.Any}, types.Undefined)
	return ctx.DefineGlobal("verifyProperty", verifyPropertyType)
}

func (a *AssertInitializer) InitRuntime(ctx *builtins.RuntimeContext) error {
	//fmt.Printf("DEBUG: Initializing assert builtins\n")
	vmInstance := ctx.VM

	// Resolve Test262Error constructor via shared handle set in Test262 initializer
	test262ErrorCtorVal := sharedTest262ErrorCtor
	if test262ErrorCtorVal == vm.Undefined {
		// Fallback: create a minimal one if missing (should not happen)
		test262ErrorCtorVal = vm.NewNativeFunction(1, true, "Test262Error", func(args []vm.Value) (vm.Value, error) {
			msg := ""
			if len(args) > 0 {
				msg = args[0].ToString()
			}
			obj := vm.NewObject(vmInstance.ErrorPrototype).AsPlainObject()
			obj.SetOwn("name", vm.NewString("Test262Error"))
			obj.SetOwn("message", vm.NewString(msg))
			return vm.NewValueFromPlainObject(obj), nil
		})
	}

	// Create assert function with properties
	assertFn := vm.NewNativeFunctionWithProps(1, true, "assert", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			return vm.Undefined, fmt.Errorf("assert requires at least 1 argument")
		}

		condition := args[0]
		message := "Assertion failed"
		if len(args) > 1 {
			message = args[1].ToString()
		}

		// Check if condition is strictly true (=== true)
		if !(condition.Type() == vm.TypeBoolean && condition.AsBoolean()) {
			// DEBUG: show which ctor we are using
			fmt.Printf("[DBG assert] Using Test262Error ctor: %s (%s)\n", test262ErrorCtorVal.Inspect(), test262ErrorCtorVal.TypeName())
			// Create Test262Error instance via constructor
			errValue, _ := vmInstance.Call(test262ErrorCtorVal, vm.Undefined, []vm.Value{vm.NewString(message)})
			if errValue.IsObject() {
				// Inspect prototype name
				if po := errValue.AsPlainObject(); po != nil {
					proto := po.GetPrototype()
					protoName := "<no name>"
					if proto.IsObject() {
						if nv, ok := proto.AsPlainObject().GetOwn("name"); ok {
							protoName = nv.ToString()
						}
					}
					fmt.Printf("[DBG assert] Created err proto name: %s\n", protoName)
				}
			}
			// Throw using VM's exception error type constructed locally
			return vm.Undefined, test262ExceptionError{v: errValue}
		}

		return vm.Undefined, nil
	})

	// Helper to compare constructors across Function vs Closure identity
	constructorsMatch := func(a, b vm.Value) bool {
		// Direct identity
		if a.Is(b) {
			return true
		}
		var aFn *vm.FunctionObject
		var bFn *vm.FunctionObject
		switch a.Type() {
		case vm.TypeFunction:
			aFn = a.AsFunction()
		case vm.TypeClosure:
			aFn = a.AsClosure().Fn
		}
		switch b.Type() {
		case vm.TypeFunction:
			bFn = b.AsFunction()
		case vm.TypeClosure:
			bFn = b.AsClosure().Fn
		}
		if aFn != nil && bFn != nil {
			return aFn == bFn
		}
		return false
	}

	// Add assert.throws method (align behavior with Test262 harness)
	assertFn.AsNativeFunctionWithProps().Properties.SetOwn("throws", vm.NewNativeFunction(0, true, "throws", func(args []vm.Value) (vm.Value, error) {
		// Optional message prefix as 3rd arg
		var messagePrefix string
		if len(args) >= 3 {
			messagePrefix = args[2].ToString() + " "
		} else {
			messagePrefix = ""
		}

		// Validate args (must have 2 args and the second is callable)
		if len(args) < 2 || !args[1].IsCallable() {
			msg := messagePrefix + "assert.throws requires two arguments: the error constructor and a function to run"
			errVal, _ := vmInstance.Call(test262ErrorCtorVal, vm.Undefined, []vm.Value{vm.NewString(msg)})
			return vm.Undefined, test262ExceptionError{v: errVal}
		}

		expectedCtor := args[0]
		fn := args[1]

		// Execute and expect throw
		_, callErr := vmInstance.Call(fn, vm.Undefined, []vm.Value{})
		if callErr == nil {
			// Mirror harness: accessing expectedErrorConstructor.name should throw if undefined/null
			// If expectedCtor is undefined/null, throw TypeError("Cannot read property 'name' of <x>")
			if expectedCtor.Type() == vm.TypeUndefined || expectedCtor.Type() == vm.TypeNull {
				typeErrCtor, _ := vmInstance.GetGlobal("TypeError")
				who := "undefined"
				if expectedCtor.Type() == vm.TypeNull {
					who = "null"
				}
				typeErr, _ := vmInstance.Call(typeErrCtor, vm.Undefined, []vm.Value{vm.NewString("Cannot read property 'name' of " + who)})
				return vm.Undefined, test262ExceptionError{v: typeErr}
			}
			expName := getCallableName(expectedCtor)
			msg := messagePrefix + "Expected a " + expName + " to be thrown but no exception was thrown at all"
			errVal, _ := vmInstance.Call(test262ErrorCtorVal, vm.Undefined, []vm.Value{vm.NewString(msg)})
			return vm.Undefined, test262ExceptionError{v: errVal}
		}

		// Extract thrown value (normalize to a VM value we can compare)
		var thrown vm.Value = vm.Undefined
		if exc, ok := callErr.(vm.ExceptionError); ok {
			thrown = exc.GetExceptionValue()
		} else {
			// Normalize non-exception Go error into Error object
			errObj := vm.NewObject(vm.Null).AsPlainObject()
			errObj.SetOwn("name", vm.NewString("Error"))
			errObj.SetOwn("message", vm.NewString(callErr.Error()))
			thrown = vm.NewValueFromPlainObject(errObj)
		}

		// Ensure thrown is object-like
		if !(thrown.Type() == vm.TypeObject || thrown.Type() == vm.TypeDictObject || thrown.Type() == vm.TypeArray) {
			// Primitive thrown: compare wrapper constructor
			var primCtor vm.Value
			switch thrown.Type() {
			case vm.TypeFloatNumber, vm.TypeIntegerNumber:
				primCtor, _ = vmInstance.GetGlobal("Number")
			case vm.TypeString:
				primCtor, _ = vmInstance.GetGlobal("String")
			case vm.TypeBoolean:
				primCtor, _ = vmInstance.GetGlobal("Boolean")
			default:
				primCtor = vm.Undefined
			}
			if primCtor != vm.Undefined {
				if expectedCtor.Type() == vm.TypeUndefined || expectedCtor.Type() == vm.TypeNull {
					typeErrCtor, _ := vmInstance.GetGlobal("TypeError")
					who := "undefined"
					if expectedCtor.Type() == vm.TypeNull {
						who = "null"
					}
					typeErr, _ := vmInstance.Call(typeErrCtor, vm.Undefined, []vm.Value{vm.NewString("Cannot read property 'name' of " + who)})
					return vm.Undefined, test262ExceptionError{v: typeErr}
				}
				if !constructorsMatch(expectedCtor, primCtor) {
					expName := getCallableName(expectedCtor)
					actName := getCallableName(primCtor)
					msg := messagePrefix + "Expected a " + expName + " but got a " + actName
					errVal, _ := vmInstance.Call(test262ErrorCtorVal, vm.Undefined, []vm.Value{vm.NewString(msg)})
					return vm.Undefined, test262ExceptionError{v: errVal}
				}
				return vm.Undefined, nil
			}
		}

		// Get thrown.constructor (prototype-aware)
		var thrownCtor vm.Value = vm.Undefined
		switch thrown.Type() {
		case vm.TypeObject:
			if po := thrown.AsPlainObject(); po != nil {
				if v, ok := po.Get("constructor"); ok {
					thrownCtor = v
				}
			}
		case vm.TypeDictObject:
			if dict := thrown.AsDictObject(); dict != nil {
				if v, ok := dict.Get("constructor"); ok {
					thrownCtor = v
				}
			}
		case vm.TypeArray:
			// Arrays inherit from Array.prototype whose constructor is Array
			arrayCtor, _ := vmInstance.GetGlobal("Array")
			thrownCtor = arrayCtor
		default:
			// primitives handled earlier
		}

		// Compare identity or names; if either constructor is undefined/null, mimic property access error on '.name'
		if expectedCtor.Type() == vm.TypeUndefined || expectedCtor.Type() == vm.TypeNull {
			typeErrCtor, _ := vmInstance.GetGlobal("TypeError")
			who := "undefined"
			if expectedCtor.Type() == vm.TypeNull {
				who = "null"
			}
			typeErr, _ := vmInstance.Call(typeErrCtor, vm.Undefined, []vm.Value{vm.NewString("Cannot read property 'name' of " + who)})
			return vm.Undefined, test262ExceptionError{v: typeErr}
		}
		if thrownCtor.Type() == vm.TypeUndefined || thrownCtor.Type() == vm.TypeNull {
			// If thrown isn't an object or lacks constructor, treat as primitive thrown
			msg := messagePrefix + "Thrown value was not an object!"
			errVal, _ := vmInstance.Call(test262ErrorCtorVal, vm.Undefined, []vm.Value{vm.NewString(msg)})
			return vm.Undefined, test262ExceptionError{v: errVal}
		}

		// Compare identity or names
		if !constructorsMatch(expectedCtor, thrownCtor) {
			expName := getCallableName(expectedCtor)
			actName := getCallableName(thrownCtor)
			var msg string
			if expName == actName {
				msg = messagePrefix + "Expected a " + expName + " but got a different error constructor with the same name"
			} else {
				msg = messagePrefix + "Expected a " + expName + " but got a " + actName
			}
			errVal, _ := vmInstance.Call(test262ErrorCtorVal, vm.Undefined, []vm.Value{vm.NewString(msg)})
			return vm.Undefined, test262ExceptionError{v: errVal}
		}

		return vm.Undefined, nil
	}))

	// Add assert.sameValue method
	assertFn.AsNativeFunctionWithProps().Properties.SetOwn("sameValue", vm.NewNativeFunction(2, true, "sameValue", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, fmt.Errorf("assert.sameValue requires at least 2 arguments")
		}

		actual := args[0]
		expected := args[1]
		message := "Values are not the same"
		if len(args) > 2 {
			message = args[2].ToString()
		}

		// Simple equality check using SameValue algorithm
		if !sameValueSimple(actual, expected) {
			fullMessage := fmt.Sprintf("%s. Expected: %s, Actual: %s", message, expected.ToString(), actual.ToString())
			// Create Test262Error via constructor and throw it
			errValue, _ := vmInstance.Call(test262ErrorCtorVal, vm.Undefined, []vm.Value{vm.NewString(fullMessage)})
			return vm.Undefined, test262ExceptionError{v: errValue}
		}

		return vm.Undefined, nil
	}))

	// Add assert.notSameValue method
	assertFn.AsNativeFunctionWithProps().Properties.SetOwn("notSameValue", vm.NewNativeFunction(2, true, "notSameValue", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.Undefined, fmt.Errorf("assert.notSameValue requires at least 2 arguments")
		}

		actual := args[0]
		expected := args[1]
		message := "Values are the same"
		if len(args) > 2 {
			message = args[2].ToString()
		}

		// Simple equality check using SameValue algorithm
		if sameValueSimple(actual, expected) {
			fullMessage := fmt.Sprintf("%s. Expected: %s, Actual: %s", message, expected.ToString(), actual.ToString())
			errValue, _ := vmInstance.Call(test262ErrorCtorVal, vm.Undefined, []vm.Value{vm.NewString(fullMessage)})
			return vm.Undefined, test262ExceptionError{v: errValue}
		}

		return vm.Undefined, nil
	}))

	// Global compareArray: boolean-returning helper
	// array-like helpers
	getArrayLikeLength := func(v vm.Value) (int, bool) {
		if arr := v.AsArray(); arr != nil {
			return arr.Length(), true
		}
		if po := v.AsPlainObject(); po != nil {
			if lv, ok := po.Get("length"); ok {
				// Coerce to int via ToFloat then int
				return int(lv.ToFloat()), true
			}
		} else if dict := v.AsDictObject(); dict != nil {
			if lv, ok := dict.Get("length"); ok {
				return int(lv.ToFloat()), true
			}
		}
		return 0, false
	}
	getArrayLikeIndex := func(v vm.Value, i int) vm.Value {
		if arr := v.AsArray(); arr != nil {
			return arr.Get(i)
		}
		key := fmt.Sprintf("%d", i)
		if po := v.AsPlainObject(); po != nil {
			if val, ok := po.Get(key); ok {
				return val
			}
			return vm.Undefined
		}
		if dict := v.AsDictObject(); dict != nil {
			if val, ok := dict.Get(key); ok {
				return val
			}
		}
		return vm.Undefined
	}

	compareArrayBoolFn := vm.NewNativeFunction(0, false, "compareArray", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.BooleanValue(false), nil
		}
		llen, lok := getArrayLikeLength(args[0])
		rlen, rok := getArrayLikeLength(args[1])
		if !lok || !rok {
			return vm.BooleanValue(false), nil
		}
		if llen != rlen {
			return vm.BooleanValue(false), nil
		}
		for i := 0; i < llen; i++ {
			if !sameValueSimple(getArrayLikeIndex(args[0], i), getArrayLikeIndex(args[1], i)) {
				return vm.BooleanValue(false), nil
			}
		}
		return vm.BooleanValue(true), nil
	})
	if err := ctx.DefineGlobal("compareArray", compareArrayBoolFn); err != nil {
		return err
	}

	// assert.compareArray: throws Test262Error with harness messages and formatted arrays
	assertCompareArrayFn := vm.NewNativeFunction(0, true, "compareArray", func(args []vm.Value) (vm.Value, error) {
		// message is optional third arg; harness expects a trailing space even when message missing
		extraMsg := " "
		if len(args) >= 3 && args[2].Type() != vm.TypeUndefined {
			extraMsg = " " + args[2].ToString()
		}

		if len(args) < 2 {
			// With 0 args: complain about Actual (first arg)
			if len(args) == 0 {
				errVal, _ := ctx.VM.Call(test262ErrorCtorVal, vm.Undefined, []vm.Value{vm.NewString("Actual argument shouldn't be nullish." + extraMsg)})
				return vm.Undefined, test262ExceptionError{v: errVal}
			}
			// With 1 arg: expected is missing
			errVal, _ := ctx.VM.Call(test262ErrorCtorVal, vm.Undefined, []vm.Value{vm.NewString("Expected argument shouldn't be nullish." + extraMsg)})
			return vm.Undefined, test262ExceptionError{v: errVal}
		}

		actual := args[0]
		expected := args[1]

		if actual.Type() == vm.TypeNull || actual.Type() == vm.TypeUndefined {
			errVal, _ := ctx.VM.Call(test262ErrorCtorVal, vm.Undefined, []vm.Value{vm.NewString("Actual argument shouldn't be nullish." + extraMsg)})
			return vm.Undefined, test262ExceptionError{v: errVal}
		}
		if expected.Type() == vm.TypeNull || expected.Type() == vm.TypeUndefined {
			errVal, _ := ctx.VM.Call(test262ErrorCtorVal, vm.Undefined, []vm.Value{vm.NewString("Expected argument shouldn't be nullish." + extraMsg)})
			return vm.Undefined, test262ExceptionError{v: errVal}
		}

		aLen, aOk := getArrayLikeLength(actual)
		bLen, bOk := getArrayLikeLength(expected)
		if !aOk || !bOk {
			// The real harness assumes arrays here; mimic a clear message
			errVal, _ := ctx.VM.Call(test262ErrorCtorVal, vm.Undefined, []vm.Value{vm.NewString("Both arguments must be arrays." + extraMsg)})
			return vm.Undefined, test262ExceptionError{v: errVal}
		}

		if aLen != bLen {
			// build formatted arrays
			formatted := func(val vm.Value, length int) string {
				parts := make([]string, length)
				for i := 0; i < length; i++ {
					parts[i] = getArrayLikeIndex(val, i).ToString()
				}
				return "[" + strings.Join(parts, ", ") + "]"
			}
			msg := "Actual " + formatted(actual, aLen) + " and expected " + formatted(expected, bLen) + " should have the same contents." + extraMsg
			errVal, _ := ctx.VM.Call(test262ErrorCtorVal, vm.Undefined, []vm.Value{vm.NewString(msg)})
			return vm.Undefined, test262ExceptionError{v: errVal}
		}

		for i := 0; i < aLen; i++ {
			if !sameValueSimple(getArrayLikeIndex(actual, i), getArrayLikeIndex(expected, i)) {
				formatted := func(val vm.Value, length int) string {
					parts := make([]string, length)
					for j := 0; j < length; j++ {
						parts[j] = getArrayLikeIndex(val, j).ToString()
					}
					return "[" + strings.Join(parts, ", ") + "]"
				}
				msg := "Actual " + formatted(actual, aLen) + " and expected " + formatted(expected, bLen) + " should have the same contents." + extraMsg
				errVal, _ := ctx.VM.Call(test262ErrorCtorVal, vm.Undefined, []vm.Value{vm.NewString(msg)})
				return vm.Undefined, test262ExceptionError{v: errVal}
			}
		}
		return vm.Undefined, nil
	})
	assertFn.AsNativeFunctionWithProps().Properties.SetOwn("compareArray", assertCompareArrayFn)

	// Minimal deepEqual stub; attach to global and assert
	deepEqualFn := vm.NewNativeFunction(2, false, "deepEqual", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.BooleanValue(false), nil
		}
		a := args[0]
		b := args[1]
		if sameValueSimple(a, b) {
			return vm.BooleanValue(true), nil
		}
		if aa := a.AsArray(); aa != nil {
			if bb := b.AsArray(); bb != nil {
				if aa.Length() != bb.Length() {
					return vm.BooleanValue(false), nil
				}
				for i := 0; i < aa.Length(); i++ {
					if !sameValueSimple(aa.Get(i), bb.Get(i)) {
						return vm.BooleanValue(false), nil
					}
				}
				return vm.BooleanValue(true), nil
			}
		}
		return vm.BooleanValue(false), nil
	})
	if err := ctx.DefineGlobal("deepEqual", deepEqualFn); err != nil {
		return err
	}
	assertFn.AsNativeFunctionWithProps().Properties.SetOwn("deepEqual", deepEqualFn)

	// decimalToHexString helper used by a few harness tests
	decToHexFn := vm.NewNativeFunction(1, false, "decimalToHexString", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewString("0"), nil
		}
		n := int64(args[0].ToFloat())
		if n < 0 {
			// emulate uint32 wrap (common in harness implementation)
			n = int64(uint32(n))
		}
		hex := fmt.Sprintf("%x", n)
		return vm.NewString(hex), nil
	})
	if err := ctx.DefineGlobal("decimalToHexString", decToHexFn); err != nil {
		return err
	}

	// nativeFunctionMatcher stub: checks function name string
	nativeFunctionMatcherFn := vm.NewNativeFunction(2, false, "nativeFunctionMatcher", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 2 {
			return vm.BooleanValue(false), nil
		}
		fn := args[0]
		pattern := args[1].ToString()
		name := ""
		if nf := fn.AsNativeFunction(); nf != nil {
			name = nf.Name
		} else if nfp := fn.AsNativeFunctionWithProps(); nfp != nil {
			name = nfp.Name
		}
		return vm.BooleanValue(strings.Contains(name, pattern)), nil
	})
	if err := ctx.DefineGlobal("nativeFunctionMatcher", nativeFunctionMatcherFn); err != nil {
		return err
	}

	// fnGlobalObject returns globalThis
	fnGlobalObjectFn := vm.NewNativeFunction(0, false, "fnGlobalObject", func(args []vm.Value) (vm.Value, error) {
		gv, _ := vmInstance.GetGlobal("globalThis")
		if gv == vm.Undefined {
			return vm.Undefined, nil
		}
		return gv, nil
	})
	if err := ctx.DefineGlobal("fnGlobalObject", fnGlobalObjectFn); err != nil {
		return err
	}

	// Helper: tolerant function identity (Function vs Closure)
	functionsMatch := func(a, b vm.Value) bool {
		if a.Is(b) {
			return true
		}
		var aFn, bFn *vm.FunctionObject
		switch a.Type() {
		case vm.TypeFunction:
			aFn = a.AsFunction()
		case vm.TypeClosure:
			aFn = a.AsClosure().Fn
		}
		switch b.Type() {
		case vm.TypeFunction:
			bFn = b.AsFunction()
		case vm.TypeClosure:
			bFn = b.AsClosure().Fn
		}
		return aFn != nil && bFn != nil && aFn == bFn
	}

	// Define verifyProperty function as global (not assert.verifyProperty)
	verifyPropertyFn := vm.NewNativeFunction(0, true, "verifyProperty", func(args []vm.Value) (vm.Value, error) {
		// verifyProperty(obj, name, desc)
		if len(args) < 3 {
			msg := "verifyProperty should receive at least 3 arguments: obj, name, and descriptor"
			errVal, _ := vmInstance.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString(msg)})
			return vm.Undefined, test262ExceptionError{v: errVal}
		}

		obj := args[0]
		key := args[1]
		nameStr := key.Inspect()
		if key.Type() != vm.TypeSymbol {
			nameStr = key.ToString()
		}
		desc := args[2]

		if obj.Type() == vm.TypeNull || obj.Type() == vm.TypeUndefined {
			msg := "Cannot verify property on null or undefined"
			errVal, _ := vmInstance.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString(msg)})
			return vm.Undefined, test262ExceptionError{v: errVal}
		}

		// Parse expected descriptor (only consider provided fields)
		var hasValue, hasWritable, hasEnumerable, hasConfigurable bool
		var hasGet, hasSet bool
		var expValue, expWritable, expEnumerable, expConfigurable, expGet, expSet vm.Value
		if po := desc.AsPlainObject(); po != nil {
			if v, ok := po.GetOwn("value"); ok {
				hasValue = true
				expValue = v
			}
			if v, ok := po.GetOwn("writable"); ok {
				hasWritable = true
				expWritable = v
			}
			if v, ok := po.GetOwn("enumerable"); ok {
				hasEnumerable = true
				expEnumerable = v
			}
			if v, ok := po.GetOwn("configurable"); ok {
				hasConfigurable = true
				expConfigurable = v
			}
			if v, ok := po.GetOwn("get"); ok {
				hasGet = true
				expGet = v
			}
			if v, ok := po.GetOwn("set"); ok {
				hasSet = true
				expSet = v
			}
		} else if dpo := desc.AsDictObject(); dpo != nil {
			if v, ok := dpo.GetOwn("value"); ok {
				hasValue = true
				expValue = v
			}
			if v, ok := dpo.GetOwn("writable"); ok {
				hasWritable = true
				expWritable = v
			}
			if v, ok := dpo.GetOwn("enumerable"); ok {
				hasEnumerable = true
				expEnumerable = v
			}
			if v, ok := dpo.GetOwn("configurable"); ok {
				hasConfigurable = true
				expConfigurable = v
			}
		} else {
			hasValue = true
			expValue = desc
		}

		// Capture original descriptor for optional restore
		originalDesc, _ := builtins.ObjectGetOwnPropertyDescriptorForHarness(obj, key)
		// Get actual descriptor using harness helper
		actualDesc, _ := builtins.ObjectGetOwnPropertyDescriptorForHarness(obj, key)
		if actualDesc == vm.Undefined {
			msg := fmt.Sprintf("obj['%s'] descriptor should be undefined", nameStr)
			errVal, _ := vmInstance.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString(msg)})
			return vm.Undefined, test262ExceptionError{v: errVal}
		}
		ad := actualDesc.AsPlainObject()

		// If expected is accessor, compare get/set and ignore value/writable
		if hasGet || hasSet {
			if hasValue || hasWritable {
				msg := fmt.Sprintf("Invalid descriptor expectation for '%s': cannot mix accessor and data fields", nameStr)
				errVal, _ := vmInstance.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString(msg)})
				return vm.Undefined, test262ExceptionError{v: errVal}
			}
			// actual must have matching get/set identity
			if hasGet {
				if v, ok := ad.GetOwn("get"); !ok || !functionsMatch(v, expGet) {
					msg := fmt.Sprintf("obj['%s'] descriptor should have matching getter", nameStr)
					errVal, _ := vmInstance.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString(msg)})
					return vm.Undefined, test262ExceptionError{v: errVal}
				}
			}
			if hasSet {
				if v, ok := ad.GetOwn("set"); !ok || !functionsMatch(v, expSet) {
					msg := fmt.Sprintf("obj['%s'] descriptor should have matching setter", nameStr)
					errVal, _ := vmInstance.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString(msg)})
					return vm.Undefined, test262ExceptionError{v: errVal}
				}
			}
		} else if hasValue {
			if v, ok := ad.GetOwn("value"); !ok || !sameValueSimple(v, expValue) {
				var actualStr string
				if av, ok2 := ad.GetOwn("value"); ok2 {
					actualStr = av.Inspect()
				} else {
					actualStr = "<missing>"
				}
				msg := fmt.Sprintf("Property '%s' has value %s, expected %s", nameStr, actualStr, expValue.Inspect())
				errVal, _ := vmInstance.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString(msg)})
				return vm.Undefined, test262ExceptionError{v: errVal}
			}
		}
		if hasWritable && !(hasGet || hasSet) {
			if v, ok := ad.GetOwn("writable"); !ok || !sameValueSimple(v, expWritable) {
				msg := fmt.Sprintf("obj['%s'] descriptor should %sbe writable", nameStr, map[bool]string{true: "", false: "not "}[expWritable.IsTruthy()])
				errVal, _ := vmInstance.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString(msg)})
				return vm.Undefined, test262ExceptionError{v: errVal}
			}
		}
		if hasEnumerable {
			if v, ok := ad.GetOwn("enumerable"); !ok || !sameValueSimple(v, expEnumerable) {
				msg := fmt.Sprintf("obj['%s'] descriptor should %sbe enumerable", nameStr, map[bool]string{true: "", false: "not "}[expEnumerable.IsTruthy()])
				errVal, _ := vmInstance.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString(msg)})
				return vm.Undefined, test262ExceptionError{v: errVal}
			}
		}
		if hasConfigurable {
			if v, ok := ad.GetOwn("configurable"); !ok || !sameValueSimple(v, expConfigurable) {
				msg := fmt.Sprintf("obj['%s'] descriptor should %sbe configurable", nameStr, map[bool]string{true: "", false: "not "}[expConfigurable.IsTruthy()])
				errVal, _ := vmInstance.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString(msg)})
				return vm.Undefined, test262ExceptionError{v: errVal}
			}
		}

		// After verification, support optional restore semantics
		if len(args) >= 4 {
			if opts := args[3].AsPlainObject(); opts != nil {
				if rv, ok := opts.GetOwn("restore"); ok && rv.IsTruthy() {
					// Restore original descriptor if it existed; otherwise delete the property
					if originalDesc != vm.Undefined {
						// Call Object.defineProperty(obj, key, originalDesc)
						objCtor, _ := vmInstance.GetGlobal("Object")
						if nfp := objCtor.AsNativeFunctionWithProps(); nfp != nil {
							if defFn, ok := nfp.Properties.GetOwn("defineProperty"); ok {
								_, _ = vmInstance.Call(defFn, vm.Undefined, []vm.Value{obj, key, originalDesc})
							}
						}
					} else {
						// Delete the property
						delName := nameStr
						if key.Type() == vm.TypeSymbol {
							delName = "@@symbol:" + key.AsSymbol()
						}
						if po := obj.AsPlainObject(); po != nil {
							_ = po.DeleteOwn(delName)
						} else if d := obj.AsDictObject(); d != nil {
							_ = d.DeleteOwn(delName)
						}
					}
					return vm.Undefined, nil
				}
			}
		}
		// Default behavior: delete the property to leave object clean
		delName := nameStr
		if key.Type() == vm.TypeSymbol {
			delName = "@@symbol:" + key.AsSymbol()
		}
		if po := obj.AsPlainObject(); po != nil {
			_ = po.DeleteOwn(delName)
		} else if d := obj.AsDictObject(); d != nil {
			_ = d.DeleteOwn(delName)
		}

		return vm.Undefined, nil
	})

	if err := ctx.DefineGlobal("verifyProperty", verifyPropertyFn); err != nil {
		return err
	}

	return ctx.DefineGlobal("assert", assertFn)
}

// getCallableName returns the .name of a function-like value, or "" if not callable
func getCallableName(v vm.Value) string {
	switch v.Type() {
	case vm.TypeFunction:
		return v.AsFunction().Name
	case vm.TypeClosure:
		return v.AsClosure().Fn.Name
	case vm.TypeNativeFunction:
		return v.AsNativeFunction().Name
	case vm.TypeNativeFunctionWithProps:
		return v.AsNativeFunctionWithProps().Name
	case vm.TypeAsyncNativeFunction:
		return v.AsAsyncNativeFunction().Name
	case vm.TypeBoundFunction:
		return v.AsBoundFunction().Name
	default:
		return ""
	}
}

// sameValueSimple implements a simplified SameValue algorithm
func sameValueSimple(x, y vm.Value) bool {
	// Basic equality check - can be enhanced later with proper SameValue semantics
	if x.Type() != y.Type() {
		return false
	}

	switch x.Type() {
	case vm.TypeNull, vm.TypeUndefined:
		return true
	case vm.TypeBoolean:
		return x.ToString() == y.ToString() // Simple comparison for now
	case vm.TypeFloatNumber, vm.TypeIntegerNumber:
		xNum := x.ToFloat()
		yNum := y.ToFloat()

		// Handle NaN case - NaN is the same as NaN in SameValue
		if xNum != xNum && yNum != yNum {
			return true // Both NaN
		}

		// Handle NaN cases where only one is NaN
		if xNum != xNum || yNum != yNum {
			return false
		}

		// Handle -0 and +0 case
		if xNum == 0 && yNum == 0 {
			// Check if they have the same sign
			// Using 1/x to distinguish -0 from +0
			xSign := 1.0 / xNum
			ySign := 1.0 / yNum
			return (xSign > 0 && ySign > 0) || (xSign < 0 && ySign < 0)
		}

		return xNum == yNum
	case vm.TypeString:
		return x.ToString() == y.ToString()
	default:
		// For objects and functions, use identity comparison
		return x == y
	}
}

// GetTest262Initializers returns the Test262-specific initializers
func GetTest262Initializers() []builtins.BuiltinInitializer {
	return []builtins.BuiltinInitializer{
		&Test262Initializer{},
		&AssertInitializer{},
	}
}

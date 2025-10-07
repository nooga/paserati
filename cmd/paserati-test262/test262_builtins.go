package main

import (
	"fmt"
	"paserati/pkg/builtins"
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

// Minimal Test262 builtins - only the essentials that harness files don't provide
// Everything else is loaded from harness JS files (sta.js, assert.js, and includes)

// Share the constructed Test262Error constructor for $ERROR to use
var sharedTest262ErrorCtor vm.Value = vm.Undefined

// test262ExceptionError adapts a VM Value into an ExceptionError for throwing from builtins
type test262ExceptionError struct{ v vm.Value }

func (e test262ExceptionError) Error() string               { return "VM exception" }
func (e test262ExceptionError) GetExceptionValue() vm.Value { return e.v }

// Test262Initializer provides minimal Test262-specific globals
type Test262Initializer struct{}

func (t *Test262Initializer) Name() string {
	return "Test262Minimal"
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

	// getWellKnownIntrinsicObject(name: string): any
	getIntrinsicType := types.NewSimpleFunction([]types.Type{types.String}, types.Any)
	if err := ctx.DefineGlobal("getWellKnownIntrinsicObject", getIntrinsicType); err != nil {
		return err
	}

	return nil
}

func (t *Test262Initializer) InitRuntime(ctx *builtins.RuntimeContext) error {
	// print function for test output
	printFn := vm.NewNativeFunctionWithProps(0, true, "print", func(args []vm.Value) (vm.Value, error) {
		parts := make([]string, len(args))
		for i, arg := range args {
			parts[i] = arg.Inspect()
		}
		if len(parts) > 0 {
			fmt.Println(parts[0])
			for i := 1; i < len(parts); i++ {
				fmt.Print(" ", parts[i])
			}
			if len(parts) > 1 {
				fmt.Println()
			}
		}
		return vm.Undefined, nil
	})
	if err := ctx.DefineGlobal("print", printFn); err != nil {
		return err
	}

	// Test262Error constructor - creates proper error objects
	test262ErrorProto := vm.NewObject(ctx.VM.ErrorPrototype).AsPlainObject()
	test262ErrorProto.SetOwn("name", vm.NewString("Test262Error"))
	test262ErrorProto.SetOwn("message", vm.NewString(""))

	var test262ErrorCtor vm.Value
	test262ErrorCtor = vm.NewNativeFunctionWithProps(1, true, "Test262Error", func(args []vm.Value) (vm.Value, error) {
		message := "Test262Error"
		if len(args) > 0 {
			message = args[0].ToString()
		}

		inst := vm.NewObject(vm.NewValueFromPlainObject(test262ErrorProto)).AsPlainObject()
		inst.SetOwn("name", vm.NewString("Test262Error"))
		inst.SetOwn("message", vm.NewString(message))
		inst.SetOwn("constructor", test262ErrorCtor)
		stack := ctx.VM.CaptureStackTrace()
		inst.SetOwn("stack", vm.NewString(stack))
		return vm.NewValueFromPlainObject(inst), nil
	})

	if ctorProps := test262ErrorCtor.AsNativeFunctionWithProps(); ctorProps != nil {
		ctorProps.Properties.SetOwn("prototype", vm.NewValueFromPlainObject(test262ErrorProto))
		test262ErrorProto.SetOwn("constructor", test262ErrorCtor)
	}

	sharedTest262ErrorCtor = test262ErrorCtor

	if err := ctx.DefineGlobal("Test262Error", test262ErrorCtor); err != nil {
		return err
	}

	// $ERROR function (legacy Test262 function)
	errorFn := vm.NewNativeFunctionWithProps(1, false, "$ERROR", func(args []vm.Value) (vm.Value, error) {
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

	// Minimal $262 harness object with createRealm and detachArrayBuffer
	harness262 := vm.NewObject(vm.Null).AsPlainObject()

	harness262.SetOwn("detachArrayBuffer", vm.NewNativeFunctionWithProps(1, false, "detachArrayBuffer", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.Undefined, nil
		}
		bufferVal := args[0]
		if buffer := bufferVal.AsArrayBuffer(); buffer != nil {
			buffer.Detach()
			return vm.Undefined, nil
		}
		return vm.Undefined, nil
	}))

	harness262.SetOwn("createRealm", vm.NewNativeFunctionWithProps(0, false, "createRealm", func(args []vm.Value) (vm.Value, error) {
		realm := vm.NewObject(vm.Null).AsPlainObject()
		realmGlobal := vm.NewObject(vm.Null).AsPlainObject()

		wrapCtor := func(name string) vm.Value {
			orig, _ := ctx.VM.GetGlobal(name)
			var origProto vm.Value = vm.Undefined
			if nfp := orig.AsNativeFunctionWithProps(); nfp != nil {
				if p, ok := nfp.Properties.GetOwn("prototype"); ok {
					origProto = p
				}
			}
			if origProto == vm.Undefined {
				origProto = ctx.VM.ErrorPrototype
			}
			localProto := vm.NewObject(origProto).AsPlainObject()
			localProto.SetOwn("name", vm.NewString(name))

			ctor := vm.NewNativeFunctionWithProps(-1, true, name, func(a []vm.Value) (vm.Value, error) {
				res, err := ctx.VM.Call(orig, vm.Undefined, a)
				if err != nil {
					return vm.Undefined, err
				}
				return res, nil
			})
			if withProps := ctor.AsNativeFunctionWithProps(); withProps != nil {
				withProps.Properties.SetOwn("prototype", vm.NewValueFromPlainObject(localProto))
				localProto.SetOwn("constructor", ctor)
			}
			return ctor
		}

		realmGlobal.SetOwn("Error", wrapCtor("Error"))
		realmGlobal.SetOwn("TypeError", wrapCtor("TypeError"))
		realmGlobal.SetOwn("ReferenceError", wrapCtor("ReferenceError"))
		realmGlobal.SetOwn("SyntaxError", wrapCtor("SyntaxError"))
		realmGlobal.SetOwn("EvalError", wrapCtor("EvalError"))
		realmGlobal.SetOwn("RangeError", wrapCtor("RangeError"))
		realmGlobal.SetOwn("URIError", wrapCtor("URIError"))

		realm.SetOwn("global", vm.NewValueFromPlainObject(realmGlobal))
		realm.SetOwn("evalScript", vm.NewNativeFunctionWithProps(1, false, "evalScript", func(args []vm.Value) (vm.Value, error) {
			return vm.Undefined, nil
		}))
		return vm.NewValueFromPlainObject(realm), nil
	}))

	if err := ctx.DefineGlobal("$262", vm.NewValueFromPlainObject(harness262)); err != nil {
		return err
	}

	// $DETACHBUFFER helper - relies on $262.detachArrayBuffer
	detacher := vm.NewNativeFunctionWithProps(1, false, "$DETACHBUFFER", func(args []vm.Value) (vm.Value, error) {
		var has bool
		var detach vm.Value
		if g262, ok := ctx.VM.GetGlobal("$262"); ok {
			if po := g262.AsPlainObject(); po != nil {
				if v, ok := po.GetOwn("detachArrayBuffer"); ok && v.IsCallable() {
					has = true
					detach = v
				}
			}
		}
		if !has {
			refErrCtor, _ := ctx.VM.GetGlobal("ReferenceError")
			refErr, _ := ctx.VM.Call(refErrCtor, vm.Undefined, []vm.Value{vm.NewString("$262.detachArrayBuffer is not defined")})
			return vm.Undefined, test262ExceptionError{v: refErr}
		}
		_, err := ctx.VM.Call(detach, vm.Undefined, args)
		if err != nil {
			return vm.Undefined, err
		}
		return vm.Undefined, nil
	})
	if err := ctx.DefineGlobal("$DETACHBUFFER", detacher); err != nil {
		return err
	}

	// getWellKnownIntrinsicObject harness helper
	getIntrinsic := vm.NewNativeFunctionWithProps(1, false, "getWellKnownIntrinsicObject", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 1 {
			errVal, _ := ctx.VM.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString("getWellKnownIntrinsicObject requires 1 argument")})
			return vm.Undefined, test262ExceptionError{v: errVal}
		}
		name := args[0].ToString()
		// Accessible intrinsics
		switch name {
		case "%Array%":
			v, _ := ctx.VM.GetGlobal("Array")
			return v, nil
		case "%Object%":
			v, _ := ctx.VM.GetGlobal("Object")
			return v, nil
		case "%Function%":
			v, _ := ctx.VM.GetGlobal("Function")
			return v, nil
		case "%Error%":
			v, _ := ctx.VM.GetGlobal("Error")
			return v, nil
		case "%TypeError%":
			v, _ := ctx.VM.GetGlobal("TypeError")
			return v, nil
		case "%RangeError%":
			v, _ := ctx.VM.GetGlobal("RangeError")
			return v, nil
		case "%ReferenceError%":
			v, _ := ctx.VM.GetGlobal("ReferenceError")
			return v, nil
		case "%SyntaxError%":
			v, _ := ctx.VM.GetGlobal("SyntaxError")
			return v, nil
		case "%EvalError%":
			v, _ := ctx.VM.GetGlobal("EvalError")
			return v, nil
		case "%URIError%":
			v, _ := ctx.VM.GetGlobal("URIError")
			return v, nil
		case "%Map%":
			v, _ := ctx.VM.GetGlobal("Map")
			return v, nil
		case "%Set%":
			v, _ := ctx.VM.GetGlobal("Set")
			return v, nil
		case "%RegExp%":
			v, _ := ctx.VM.GetGlobal("RegExp")
			return v, nil
		case "%ArrayBuffer%":
			v, _ := ctx.VM.GetGlobal("ArrayBuffer")
			return v, nil
		case "%DataView%":
			v, _ := ctx.VM.GetGlobal("DataView")
			return v, nil
		case "%Promise%":
			v, _ := ctx.VM.GetGlobal("Promise")
			return v, nil
		case "%Symbol%":
			v, _ := ctx.VM.GetGlobal("Symbol")
			return v, nil
		case "%BigInt%":
			v, _ := ctx.VM.GetGlobal("BigInt")
			return v, nil
		}
		// Known but inaccessible intrinsics
		switch name {
		case "%AsyncFromSyncIteratorPrototype%",
			"%IteratorPrototype%",
			"%TypedArray%",
			"%ArrayIteratorPrototype%",
			"%StringIteratorPrototype%",
			"%MapIteratorPrototype%",
			"%SetIteratorPrototype%":
			errVal, _ := ctx.VM.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString("intrinsic not accessible")})
			return vm.Undefined, test262ExceptionError{v: errVal}
		}
		// Unknown intrinsic
		errVal, _ := ctx.VM.Call(sharedTest262ErrorCtor, vm.Undefined, []vm.Value{vm.NewString("intrinsic not found")})
		return vm.Undefined, test262ExceptionError{v: errVal}
	})
	if err := ctx.DefineGlobal("getWellKnownIntrinsicObject", getIntrinsic); err != nil {
		return err
	}

	// Provide minimal native Error subclasses missing from engine
	makeErrorSubclass := func(name string) vm.Value {
		proto := vm.NewObject(ctx.VM.ErrorPrototype).AsPlainObject()
		proto.SetOwn("name", vm.NewString(name))
		ctor := vm.NewNativeFunctionWithProps(1, true, name, func(args []vm.Value) (vm.Value, error) {
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
		if withProps := ctor.AsNativeFunctionWithProps(); withProps != nil {
			withProps.Properties.SetOwn("prototype", vm.NewValueFromPlainObject(proto))
			proto.SetOwn("constructor", ctor)
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
	if err := ctx.DefineGlobal("ReferenceError", makeErrorSubclass("ReferenceError")); err != nil {
		return err
	}

	return nil
}

// GetTest262Initializers returns the minimal Test262-specific initializers
func GetTest262Initializers() []builtins.BuiltinInitializer {
	return []builtins.BuiltinInitializer{
		&Test262Initializer{},
	}
}

package main

import (
	"fmt"

	"github.com/nooga/paserati/pkg/builtins"
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// Minimal Test262 builtins - only the essentials that harness files don't provide
// Everything else is loaded from harness JS files (sta.js, assert.js, and includes)

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

	// NOTE: Test262Error is defined in sta.js, not here
	// NOTE: $ERROR is defined in sta.js, not here

	// getWellKnownIntrinsicObject(name: string): any
	getIntrinsicType := types.NewSimpleFunction([]types.Type{types.String}, types.Any)
	if err := ctx.DefineGlobal("getWellKnownIntrinsicObject", getIntrinsicType); err != nil {
		return err
	}

	// Define harness globals as Any to satisfy static checker
	// The actual implementation is provided by harness files (assert.js, sta.js)
	// This is critical for module tests where dependencies (self-imports) are checked
	// by a fresh checker instance that doesn't see the harness script's scope.
	if err := ctx.DefineGlobal("assert", types.Any); err != nil {
		return err
	}
	if err := ctx.DefineGlobal("Test262Error", types.Any); err != nil {
		return err
	}
	if err := ctx.DefineGlobal("$DONOTEVALUATE", types.Any); err != nil {
		return err
	}
	if err := ctx.DefineGlobal("$ERROR", types.Any); err != nil {
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

	// NOTE: Test262Error and $ERROR are defined in sta.js, not here

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

		// Create a realm-local error constructor that creates instances with
		// the realm's own prototype, not the main realm's prototype.
		// This is critical for cross-realm instanceof/constructor checks.
		makeRealmErrorCtor := func(name string) vm.Value {
			// Get the original prototype to inherit from (for proper prototype chain)
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

			// Create a local prototype that inherits from the original
			localProto := vm.NewObject(origProto).AsPlainObject()
			localProto.SetOwn("name", vm.NewString(name))

			// The constructor must create instances with localProto, NOT call the original
			// Use NewConstructorWithProps to ensure IsConstructor is true
			var ctor vm.Value
			ctor = vm.NewConstructorWithProps(-1, true, name, func(a []vm.Value) (vm.Value, error) {
				msg := ""
				if len(a) > 0 && a[0].Type() != vm.TypeUndefined {
					msg = a[0].ToString()
				}
				// Create instance with the realm's local prototype
				inst := vm.NewObject(vm.NewValueFromPlainObject(localProto)).AsPlainObject()
				// Set [[ErrorData]] internal slot (used by Error.isError to distinguish real errors)
				inst.SetOwn("[[ErrorData]]", vm.Undefined)
				inst.SetOwn("name", vm.NewString(name))
				inst.SetOwn("message", vm.NewString(msg))
				inst.SetOwn("stack", vm.NewString(ctx.VM.CaptureStackTrace()))
				return vm.NewValueFromPlainObject(inst), nil
			})
			if withProps := ctor.AsNativeFunctionWithProps(); withProps != nil {
				withProps.Properties.SetOwn("prototype", vm.NewValueFromPlainObject(localProto))
				localProto.SetOwn("constructor", ctor)
			}
			return ctor
		}

		realmGlobal.SetOwn("Error", makeRealmErrorCtor("Error"))
		realmGlobal.SetOwn("TypeError", makeRealmErrorCtor("TypeError"))
		realmGlobal.SetOwn("ReferenceError", makeRealmErrorCtor("ReferenceError"))
		realmGlobal.SetOwn("SyntaxError", makeRealmErrorCtor("SyntaxError"))
		realmGlobal.SetOwn("EvalError", makeRealmErrorCtor("EvalError"))
		realmGlobal.SetOwn("RangeError", makeRealmErrorCtor("RangeError"))
		realmGlobal.SetOwn("URIError", makeRealmErrorCtor("URIError"))

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
		throwError := func(msg string) error {
			if errorCtor, ok := ctx.VM.GetGlobal("Test262Error"); ok && errorCtor.IsCallable() {
				errVal, _ := ctx.VM.Call(errorCtor, vm.Undefined, []vm.Value{vm.NewString(msg)})
				return test262ExceptionError{v: errVal}
			}
			return fmt.Errorf("%s", msg)
		}
		if len(args) < 1 {
			return vm.Undefined, throwError("getWellKnownIntrinsicObject requires 1 argument")
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
			return vm.Undefined, throwError("intrinsic not accessible")
		}
		// Unknown intrinsic
		return vm.Undefined, throwError("intrinsic not found")
	})
	if err := ctx.DefineGlobal("getWellKnownIntrinsicObject", getIntrinsic); err != nil {
		return err
	}

	// NOTE: EvalError, RangeError, URIError, and ReferenceError are now provided
	// by the standard builtins (error_init.go, reference_error_init.go).
	// No need to redefine them here.

	return nil
}

// GetTest262Initializers returns the minimal Test262-specific initializers
func GetTest262Initializers() []builtins.BuiltinInitializer {
	return []builtins.BuiltinInitializer{
		&Test262Initializer{},
	}
}

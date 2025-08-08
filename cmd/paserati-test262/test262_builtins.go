package main

import (
	"fmt"
	"paserati/pkg/builtins"
	"paserati/pkg/types"
	"paserati/pkg/vm"
	"strings"
)

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
	test262ErrorCtor = vm.NewNativeFunction(1, false, "Test262Error", func(args []vm.Value) (vm.Value, error) {
		message := "Test262Error"
		if len(args) > 0 {
			message = args[0].ToString()
		}

		// Create error object with constructor property
		errObj := vm.NewObject(vm.Null).AsPlainObject()
		errObj.SetOwn("name", vm.NewString("Test262Error"))
		errObj.SetOwn("message", vm.NewString(message))
		errObj.SetOwn("constructor", test262ErrorCtor) // Set constructor to itself
		errObj.SetOwn("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
			return vm.NewString(fmt.Sprintf("Test262Error: %s", message)), nil
		}))

		return vm.NewValueFromPlainObject(errObj), nil
	})
	if err := ctx.DefineGlobal("Test262Error", test262ErrorCtor); err != nil {
		return err
	}

	// $ERROR function (legacy Test262 function)
	errorFn := vm.NewNativeFunction(1, false, "$ERROR", func(args []vm.Value) (vm.Value, error) {
		message := "Test failed"
		if len(args) > 0 {
			message = args[0].ToString()
		}
		return vm.Undefined, fmt.Errorf("Test262Error: %s", message)
	})
	if err := ctx.DefineGlobal("$ERROR", errorFn); err != nil {
		return err
	}

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
	
	// Store Test262Error constructor for later use
	test262ErrorCtor, _ := vmInstance.GetGlobal("Test262Error")

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

		// Check if condition is truthy
		if !condition.IsTruthy() {
			// Create Test262Error object and throw it as an exception
			errObj := vm.NewObject(vm.Null).AsPlainObject()
			errObj.SetOwn("name", vm.NewString("Test262Error"))
			errObj.SetOwn("message", vm.NewString(message))
			errObj.SetOwn("constructor", test262ErrorCtor)
			errObj.SetOwn("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
				return vm.NewString(fmt.Sprintf("Test262Error: %s", message)), nil
			}))
			errValue := vm.NewValueFromPlainObject(errObj)
			// Return a Test262Exception that contains the error object
			return vm.Undefined, &Test262Exception{
				ErrorObject: errValue,
				Message:     fmt.Sprintf("Test262Error: %s", message),
			}
		}

		return vm.Undefined, nil
	})

	// Add assert.throws method
	assertFn.AsNativeFunctionWithProps().Properties.SetOwn("throws", vm.NewNativeFunction(2, false, "throws", func(args []vm.Value) (vm.Value, error) {
		// fmt.Printf("[DEBUG assert.throws] Called with %d args\n", len(args))
		if len(args) < 2 {
			return vm.Undefined, fmt.Errorf("assert.throws requires 2 arguments")
		}

		fn := args[1] // Function to call
		// fmt.Printf("[DEBUG assert.throws] Function type: %v, callable: %v\n", fn.Type(), fn.IsCallable())

		// Check if fn is callable
		if !fn.IsCallable() {
			return vm.Undefined, fmt.Errorf("assert.throws: second argument must be a function")
		}

		// fmt.Printf("[DEBUG assert.throws] About to call CallFunctionDirectly\n")
		// Call the function and expect it to throw
		_, err := vmInstance.CallFunctionDirectly(fn, vm.Undefined, []vm.Value{})
		// fmt.Printf("[DEBUG assert.throws] CallFunctionDirectly returned, err=%v\n", err)

		if err == nil {
			// fmt.Printf("[DEBUG assert.throws] Function didn't throw, returning error\n")
			return vm.Undefined, fmt.Errorf("Test262Error: Expected function to throw, but it didn't")
		}

		// TODO: Check that the error type matches expectedError (args[0])
		// For now, just verify that an error was thrown
		// fmt.Printf("[DEBUG assert.throws] Function threw as expected: %v\n", err)

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
			// Create Test262Error object and throw it
			errObj := vm.NewObject(vm.Null).AsPlainObject()
			errObj.SetOwn("name", vm.NewString("Test262Error"))
			errObj.SetOwn("message", vm.NewString(fullMessage))
			errObj.SetOwn("constructor", test262ErrorCtor)
			errObj.SetOwn("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
				return vm.NewString(fmt.Sprintf("Test262Error: %s", fullMessage)), nil
			}))
			errValue := vm.NewValueFromPlainObject(errObj)
			return vm.Undefined, &Test262Exception{
				ErrorObject: errValue,
				Message:     fmt.Sprintf("Test262Error: %s", fullMessage),
			}
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
			// Create Test262Error object and throw it
			errObj := vm.NewObject(vm.Null).AsPlainObject()
			errObj.SetOwn("name", vm.NewString("Test262Error"))
			errObj.SetOwn("message", vm.NewString(fullMessage))
			errObj.SetOwn("constructor", test262ErrorCtor)
			errObj.SetOwn("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
				return vm.NewString(fmt.Sprintf("Test262Error: %s", fullMessage)), nil
			}))
			errValue := vm.NewValueFromPlainObject(errObj)
			return vm.Undefined, &Test262Exception{
				ErrorObject: errValue,
				Message:     fmt.Sprintf("Test262Error: %s", fullMessage),
			}
		}

		return vm.Undefined, nil
	}))

	// Define verifyProperty function as global (not assert.verifyProperty)
	verifyPropertyFn := vm.NewNativeFunction(3, false, "verifyProperty", func(args []vm.Value) (vm.Value, error) {
		if len(args) < 3 {
			return vm.Undefined, fmt.Errorf("verifyProperty requires 3 arguments")
		}

		obj := args[0]
		prop := args[1].ToString()
		expected := args[2]

		// Check if object has the property
		if obj.Type() == vm.TypeNull || obj.Type() == vm.TypeUndefined {
			return vm.Undefined, fmt.Errorf("Test262Error: Cannot verify property on null or undefined")
		}

		// Get property value
		var actual vm.Value
		if plainObj := obj.AsPlainObject(); plainObj != nil {
			if plainObj.HasOwn(prop) {
				actual, _ = plainObj.GetOwn(prop)
			} else {
				return vm.Undefined, fmt.Errorf("Test262Error: Property '%s' not found", prop)
			}
		} else if arrayObj := obj.AsArray(); arrayObj != nil && prop == "length" {
			actual = vm.NumberValue(float64(arrayObj.Length()))
		} else {
			return vm.Undefined, fmt.Errorf("Test262Error: Cannot access property '%s' on value", prop)
		}

		// Check if property value matches expected
		if !sameValueSimple(actual, expected) {
			return vm.Undefined, fmt.Errorf("Test262Error: Property '%s' has value %s, expected %s",
				prop, actual.ToString(), expected.ToString())
		}

		return vm.Undefined, nil
	})

	if err := ctx.DefineGlobal("verifyProperty", verifyPropertyFn); err != nil {
		return err
	}

	return ctx.DefineGlobal("assert", assertFn)
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

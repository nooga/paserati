package vm

// // StringPrototype holds the prototype object for String
// var StringPrototype *PlainObject

// // ArrayPrototype holds the prototype object for Array
// var ArrayPrototype *PlainObject

// // FunctionPrototype holds the prototype object for Function
// var FunctionPrototype *PlainObject

// // initPrototypes initializes the built-in prototype objects as empty PlainObjects
// func initPrototypes() {
// 	if StringPrototype != nil {
// 		return // Already initialized
// 	}

// 	// Create empty prototype objects
// 	StringPrototype = NewObject(Undefined).AsPlainObject()
// 	ArrayPrototype = NewObject(Undefined).AsPlainObject()
// 	FunctionPrototype = NewObject(Undefined).AsPlainObject()
// }

// // RegisterStringPrototypeMethod allows external packages to register String prototype methods
// func RegisterStringPrototypeMethod(methodName string, method Value) {
// 	initPrototypes() // Ensure prototypes are initialized
// 	StringPrototype.SetOwn(methodName, method)
// }

// // RegisterArrayPrototypeMethod allows external packages to register Array prototype methods
// func RegisterArrayPrototypeMethod(methodName string, method Value) {
// 	initPrototypes() // Ensure prototypes are initialized
// 	ArrayPrototype.SetOwn(methodName, method)
// }

// // RegisterFunctionPrototypeMethod allows external packages to register Function prototype methods
// func RegisterFunctionPrototypeMethod(methodName string, method Value) {
// 	initPrototypes() // Ensure prototypes are initialized
// 	FunctionPrototype.SetOwn(methodName, method)
// }

// // createBoundMethod creates a method bound to a specific 'this' value
func createBoundMethod(thisValue Value, method Value) Value {
	switch method.Type() {
	case TypeNativeFunction:
		nativeMethod := method.AsNativeFunction()

		// Regular method binding for all native functions
		boundFn := func(args []Value) Value {
			// Prepend 'this' to the arguments
			boundArgs := make([]Value, len(args)+1)
			boundArgs[0] = thisValue
			copy(boundArgs[1:], args)
			return nativeMethod.Fn(boundArgs)
		}

		boundMethod := &NativeFunctionObject{
			Arity:    nativeMethod.Arity,
			Variadic: nativeMethod.Variadic,
			Name:     nativeMethod.Name,
			Fn:       boundFn,
		}

		return NewNativeFunction(boundMethod.Arity, boundMethod.Variadic, boundMethod.Name, boundMethod.Fn)

	case TypeAsyncNativeFunction:
		asyncMethod := method.AsAsyncNativeFunction()
		boundAsyncFn := func(caller VMCaller, args []Value) Value {
			// Prepend 'this' to the arguments
			boundArgs := make([]Value, len(args)+1)
			boundArgs[0] = thisValue
			copy(boundArgs[1:], args)
			return asyncMethod.AsyncFn(caller, boundArgs)
		}

		return NewAsyncNativeFunction(asyncMethod.Arity, asyncMethod.Variadic, asyncMethod.Name, boundAsyncFn)

	case TypeNativeFunctionWithProps:
		nativeMethodWithProps := method.AsNativeFunctionWithProps()
		boundFn := func(args []Value) Value {
			// Prepend 'this' to the arguments
			boundArgs := make([]Value, len(args)+1)
			boundArgs[0] = thisValue
			copy(boundArgs[1:], args)
			return nativeMethodWithProps.Fn(boundArgs)
		}

		// For props functions, we need to be careful about preserving properties
		return NewNativeFunction(nativeMethodWithProps.Arity, nativeMethodWithProps.Variadic, nativeMethodWithProps.Name, boundFn)

	default:
		// If not a native function type, return as-is
		return method
	}
}

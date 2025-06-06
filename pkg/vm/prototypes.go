package vm

// StringPrototype holds the prototype object for String
var StringPrototype *PlainObject

// ArrayPrototype holds the prototype object for Array
var ArrayPrototype *PlainObject

// initPrototypes initializes the built-in prototype objects as empty PlainObjects
func initPrototypes() {
	if StringPrototype != nil {
		return // Already initialized
	}

	// Create empty prototype objects
	StringPrototype = NewObject(Undefined).AsPlainObject()
	ArrayPrototype = NewObject(Undefined).AsPlainObject()
}

// RegisterStringPrototypeMethod allows external packages to register String prototype methods
func RegisterStringPrototypeMethod(methodName string, method Value) {
	initPrototypes() // Ensure prototypes are initialized
	StringPrototype.SetOwn(methodName, method)
}

// RegisterArrayPrototypeMethod allows external packages to register Array prototype methods
func RegisterArrayPrototypeMethod(methodName string, method Value) {
	initPrototypes() // Ensure prototypes are initialized
	ArrayPrototype.SetOwn(methodName, method)
}

// createBoundMethod creates a method bound to a specific 'this' value
func createBoundMethod(thisValue Value, method Value) Value {
	if !method.IsNativeFunction() {
		return method // If not a native function, return as-is
	}

	nativeMethod := method.AsNativeFunction()
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
}

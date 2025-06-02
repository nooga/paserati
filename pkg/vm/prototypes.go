package vm

import (
	"unsafe"
)

// StringPrototype holds the prototype methods for String
var StringPrototype map[string]*NativeFunctionObject

// ArrayPrototype holds the prototype methods for Array
var ArrayPrototype map[string]*NativeFunctionObject

// initPrototypes initializes the built-in prototypes
func initPrototypes() {
	if StringPrototype != nil {
		return // Already initialized
	}

	StringPrototype = map[string]*NativeFunctionObject{
		"charCodeAt": {
			Arity:    1,
			Variadic: false,
			Name:     "charCodeAt",
			Fn:       stringCharCodeAt,
		},
		"charAt": {
			Arity:    1,
			Variadic: false,
			Name:     "charAt",
			Fn:       stringCharAt,
		},
	}

	ArrayPrototype = map[string]*NativeFunctionObject{
		"concat": {
			Arity:    -1,
			Variadic: true,
			Name:     "concat",
			Fn:       arrayConcat,
		},
		"push": {
			Arity:    -1,
			Variadic: true,
			Name:     "push",
			Fn:       arrayPush,
		},
		"pop": {
			Arity:    0,
			Variadic: false,
			Name:     "pop",
			Fn:       arrayPop,
		},
	}
}

// String prototype method implementations

func stringCharCodeAt(args []Value) Value {
	// args[0] is 'this' (the string), args[1] is the index
	if len(args) < 2 {
		return NumberValue(float64(-1)) // NaN equivalent
	}

	str := args[0].ToString()
	index := int(args[1].ToFloat())

	if index < 0 || index >= len(str) {
		return NumberValue(float64(-1)) // NaN equivalent
	}

	return NumberValue(float64(str[index]))
}

func stringCharAt(args []Value) Value {
	// args[0] is 'this' (the string), args[1] is the index
	if len(args) < 2 {
		return NewString("")
	}

	str := args[0].ToString()
	index := int(args[1].ToFloat())

	if index < 0 || index >= len(str) {
		return NewString("")
	}

	return NewString(string(str[index]))
}

// Array prototype method implementations

func arrayConcat(args []Value) Value {
	// args[0] is 'this' (the array)
	if len(args) == 0 {
		return NewArray()
	}

	thisArray := args[0]
	if !thisArray.IsArray() {
		return NewArray()
	}

	result := NewArray()
	resultArr := result.AsArray()
	sourceArr := thisArray.AsArray()

	// Copy elements from the source array
	for i := 0; i < sourceArr.Length(); i++ {
		resultArr.Append(sourceArr.Get(i))
	}

	// Concatenate all additional arguments (starting from index 1)
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg.IsArray() {
			// If argument is an array, append all its elements
			arr := arg.AsArray()
			for j := 0; j < arr.Length(); j++ {
				resultArr.Append(arr.Get(j))
			}
		} else {
			// If argument is not an array, append it as a single element
			resultArr.Append(arg)
		}
	}

	return result
}

func arrayPush(args []Value) Value {
	// args[0] is 'this' (the array)
	if len(args) == 0 {
		return NumberValue(0)
	}

	thisArray := args[0]
	if !thisArray.IsArray() {
		return NumberValue(0)
	}

	arr := thisArray.AsArray()

	// Append all arguments starting from index 1
	for i := 1; i < len(args); i++ {
		arr.Append(args[i])
	}

	return NumberValue(float64(arr.Length()))
}

func arrayPop(args []Value) Value {
	// args[0] is 'this' (the array)
	if len(args) == 0 {
		return Undefined
	}

	thisArray := args[0]
	if !thisArray.IsArray() {
		return Undefined
	}

	arr := thisArray.AsArray()
	if arr.Length() == 0 {
		return Undefined
	}

	// Get the last element
	lastElement := arr.Get(arr.Length() - 1)

	// Reduce the length by 1
	arr.SetLength(arr.Length() - 1)

	return lastElement
}

// createBoundMethod creates a method bound to a specific 'this' value
func createBoundMethod(thisValue Value, method *NativeFunctionObject) Value {
	boundFn := func(args []Value) Value {
		// Prepend 'this' to the arguments
		boundArgs := make([]Value, len(args)+1)
		boundArgs[0] = thisValue
		copy(boundArgs[1:], args)
		return method.Fn(boundArgs)
	}

	boundMethod := &NativeFunctionObject{
		Arity:    method.Arity,
		Variadic: method.Variadic,
		Name:     method.Name,
		Fn:       boundFn,
	}

	return Value{typ: TypeNativeFunction, obj: unsafe.Pointer(boundMethod)}
}

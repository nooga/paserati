package builtins

import (
	"paserati/pkg/types"
	"paserati/pkg/vm"
	"strings"
	"unicode"
)

// registerString registers the String constructor and static methods
func registerString() {
	// Register String constructor using NativeFunctionWithProps
	stringConstructorValue := vm.NewNativeFunctionWithProps(-1, true, "String", stringConstructor)

	// Add fromCharCode as a property on the String constructor
	stringConstructorObj := stringConstructorValue.AsNativeFunctionWithProps()
	stringConstructorObj.Properties.SetOwn("fromCharCode",
		vm.NewNativeFunction(-1, true, "fromCharCode", stringFromCharCode))

	// Register with type checker using the unified ObjectType system
	stringCallableType := types.NewObjectType().
		// Call signature for String constructor
		WithCallSignature(types.SigVariadic([]types.Type{}, types.String, &types.ArrayType{ElementType: types.Any})).
		
		// Static methods on String constructor
		WithProperty("fromCharCode", types.NewVariadicFunction([]types.Type{}, types.String, &types.ArrayType{ElementType: types.Number}))

	registerValue("String", stringConstructorValue, stringCallableType)

	// Register String prototype methods (both runtime and types)
	registerStringPrototypeMethods()
}

// registerStringPrototypeMethods registers String prototype methods with both implementations and types
func registerStringPrototypeMethods() {
	// Register charAt method
	vm.RegisterStringPrototypeMethod("charAt",
		vm.NewNativeFunction(1, false, "charAt", stringCharAtImpl))
	RegisterPrototypeMethod("string", "charAt", 
		types.NewSimpleFunction([]types.Type{types.Number}, types.String))

	// Register charCodeAt method
	vm.RegisterStringPrototypeMethod("charCodeAt",
		vm.NewNativeFunction(1, false, "charCodeAt", stringCharCodeAtImpl))
	RegisterPrototypeMethod("string", "charCodeAt", 
		types.NewSimpleFunction([]types.Type{types.Number}, types.Number))

	// Register substring method
	vm.RegisterStringPrototypeMethod("substring",
		vm.NewNativeFunction(2, false, "substring", stringSubstringImpl))
	RegisterPrototypeMethod("string", "substring", 
		types.NewSignature(types.Number, types.Number).WithOptional(false, true).Returns(types.String).ToFunction())

	// Register slice method
	vm.RegisterStringPrototypeMethod("slice",
		vm.NewNativeFunction(2, false, "slice", stringSliceImpl))
	RegisterPrototypeMethod("string", "slice", 
		types.NewSignature(types.Number, types.Number).WithOptional(false, true).Returns(types.String).ToFunction())

	// Register indexOf method
	vm.RegisterStringPrototypeMethod("indexOf",
		vm.NewNativeFunction(1, false, "indexOf", stringIndexOfImpl))
	RegisterPrototypeMethod("string", "indexOf", 
		types.NewSimpleFunction([]types.Type{types.String}, types.Number))

	// Register includes method
	vm.RegisterStringPrototypeMethod("includes",
		vm.NewNativeFunction(1, false, "includes", stringIncludesImpl))
	RegisterPrototypeMethod("string", "includes", 
		types.NewSimpleFunction([]types.Type{types.String}, types.Boolean))

	// Register startsWith method
	vm.RegisterStringPrototypeMethod("startsWith",
		vm.NewNativeFunction(1, false, "startsWith", stringStartsWithImpl))
	RegisterPrototypeMethod("string", "startsWith", 
		types.NewSimpleFunction([]types.Type{types.String}, types.Boolean))

	// Register endsWith method
	vm.RegisterStringPrototypeMethod("endsWith",
		vm.NewNativeFunction(1, false, "endsWith", stringEndsWithImpl))
	RegisterPrototypeMethod("string", "endsWith", 
		types.NewSimpleFunction([]types.Type{types.String}, types.Boolean))

	// Register toLowerCase method
	vm.RegisterStringPrototypeMethod("toLowerCase",
		vm.NewNativeFunction(0, false, "toLowerCase", stringToLowerCaseImpl))
	RegisterPrototypeMethod("string", "toLowerCase", 
		types.NewSimpleFunction([]types.Type{}, types.String))

	// Register toUpperCase method
	vm.RegisterStringPrototypeMethod("toUpperCase",
		vm.NewNativeFunction(0, false, "toUpperCase", stringToUpperCaseImpl))
	RegisterPrototypeMethod("string", "toUpperCase", 
		types.NewSimpleFunction([]types.Type{}, types.String))

	// Register trim method
	vm.RegisterStringPrototypeMethod("trim",
		vm.NewNativeFunction(0, false, "trim", stringTrimImpl))
	RegisterPrototypeMethod("string", "trim", 
		types.NewSimpleFunction([]types.Type{}, types.String))

	// Register trimStart method
	vm.RegisterStringPrototypeMethod("trimStart",
		vm.NewNativeFunction(0, false, "trimStart", stringTrimStartImpl))
	RegisterPrototypeMethod("string", "trimStart", 
		types.NewSimpleFunction([]types.Type{}, types.String))

	// Register trimEnd method
	vm.RegisterStringPrototypeMethod("trimEnd",
		vm.NewNativeFunction(0, false, "trimEnd", stringTrimEndImpl))
	RegisterPrototypeMethod("string", "trimEnd", 
		types.NewSimpleFunction([]types.Type{}, types.String))

	// Register repeat method
	vm.RegisterStringPrototypeMethod("repeat",
		vm.NewNativeFunction(1, false, "repeat", stringRepeatImpl))
	RegisterPrototypeMethod("string", "repeat", 
		types.NewSimpleFunction([]types.Type{types.Number}, types.String))

	// Register lastIndexOf method
	vm.RegisterStringPrototypeMethod("lastIndexOf",
		vm.NewNativeFunction(1, false, "lastIndexOf", stringLastIndexOfImpl))
	RegisterPrototypeMethod("string", "lastIndexOf", 
		types.NewSimpleFunction([]types.Type{types.String}, types.Number))

	// Register concat method
	vm.RegisterStringPrototypeMethod("concat",
		vm.NewNativeFunction(-1, true, "concat", stringConcatImpl))
	RegisterPrototypeMethod("string", "concat", 
		types.NewVariadicFunction([]types.Type{}, types.String, &types.ArrayType{ElementType: types.String}))

	// Register split method
	vm.RegisterStringPrototypeMethod("split",
		vm.NewNativeFunction(1, false, "split", stringSplitImpl))
	RegisterPrototypeMethod("string", "split", 
		types.NewSignature(types.String).WithOptional(true).Returns(&types.ArrayType{ElementType: types.String}).ToFunction())
}

// stringCharAtImpl implements String.prototype.charAt
func stringCharAtImpl(args []vm.Value) vm.Value {
	// args[0] is 'this' (the string), args[1] is the index
	if len(args) < 2 {
		return vm.NewString("")
	}

	str := args[0].ToString()
	index := int(args[1].ToFloat())

	if index < 0 || index >= len(str) {
		return vm.NewString("")
	}

	return vm.NewString(string(str[index]))
}

// stringCharCodeAtImpl implements String.prototype.charCodeAt
func stringCharCodeAtImpl(args []vm.Value) vm.Value {
	// args[0] is 'this' (the string), args[1] is the index
	if len(args) < 2 {
		return vm.Number(float64('N')) // Default to 'N' if no args? Or NaN?
	}

	str := args[0].ToString()
	index := int(args[1].ToFloat())

	if index < 0 || index >= len(str) {
		return vm.Number(float64('?')) // Return '?' for out of bounds? Or NaN?
	}

	return vm.Number(float64(str[index]))
}

// stringSubstringImpl implements String.prototype.substring
func stringSubstringImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.NewString("")
	}

	str := args[0].ToString()
	start := int(args[1].ToFloat())

	// Default end to string length if not provided
	end := len(str)
	if len(args) > 2 && args[2].Type() != vm.TypeUndefined {
		end = int(args[2].ToFloat())
	}

	// Clamp values
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}
	if start > len(str) {
		start = len(str)
	}
	if end > len(str) {
		end = len(str)
	}

	// Swap if start > end
	if start > end {
		start, end = end, start
	}

	return vm.NewString(str[start:end])
}

// stringSliceImpl implements String.prototype.slice
func stringSliceImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.NewString("")
	}

	str := args[0].ToString()
	start := int(args[1].ToFloat())

	// Default end to string length if not provided
	end := len(str)
	if len(args) > 2 && args[2].Type() != vm.TypeUndefined {
		end = int(args[2].ToFloat())
	}

	// Handle negative indices
	if start < 0 {
		start = len(str) + start
		if start < 0 {
			start = 0
		}
	}
	if end < 0 {
		end = len(str) + end
		if end < 0 {
			end = 0
		}
	}

	// Clamp to string bounds
	if start > len(str) {
		start = len(str)
	}
	if end > len(str) {
		end = len(str)
	}

	// Return empty string if start >= end
	if start >= end {
		return vm.NewString("")
	}

	return vm.NewString(str[start:end])
}

// stringIndexOfImpl implements String.prototype.indexOf
func stringIndexOfImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.Number(-1)
	}

	str := args[0].ToString()
	searchStr := args[1].ToString()

	for i := 0; i <= len(str)-len(searchStr); i++ {
		if str[i:i+len(searchStr)] == searchStr {
			return vm.Number(float64(i))
		}
	}

	return vm.Number(-1)
}

// stringIncludesImpl implements String.prototype.includes
func stringIncludesImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.BooleanValue(false)
	}

	str := args[0].ToString()
	searchStr := args[1].ToString()

	for i := 0; i <= len(str)-len(searchStr); i++ {
		if str[i:i+len(searchStr)] == searchStr {
			return vm.BooleanValue(true)
		}
	}

	return vm.BooleanValue(false)
}

// stringStartsWithImpl implements String.prototype.startsWith
func stringStartsWithImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.BooleanValue(false)
	}

	str := args[0].ToString()
	searchStr := args[1].ToString()

	if len(searchStr) > len(str) {
		return vm.BooleanValue(false)
	}

	return vm.BooleanValue(str[:len(searchStr)] == searchStr)
}

// stringEndsWithImpl implements String.prototype.endsWith
func stringEndsWithImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.BooleanValue(false)
	}

	str := args[0].ToString()
	searchStr := args[1].ToString()

	if len(searchStr) > len(str) {
		return vm.BooleanValue(false)
	}

	return vm.BooleanValue(str[len(str)-len(searchStr):] == searchStr)
}

// stringToLowerCaseImpl implements String.prototype.toLowerCase
func stringToLowerCaseImpl(args []vm.Value) vm.Value {
	if len(args) < 1 {
		return vm.NewString("")
	}

	str := args[0].ToString()
	return vm.NewString(strings.ToLower(str))
}

// stringToUpperCaseImpl implements String.prototype.toUpperCase
func stringToUpperCaseImpl(args []vm.Value) vm.Value {
	if len(args) < 1 {
		return vm.NewString("")
	}

	str := args[0].ToString()
	return vm.NewString(strings.ToUpper(str))
}

// stringTrimImpl implements String.prototype.trim
func stringTrimImpl(args []vm.Value) vm.Value {
	if len(args) < 1 {
		return vm.NewString("")
	}

	str := args[0].ToString()
	return vm.NewString(strings.TrimSpace(str))
}

// stringTrimStartImpl implements String.prototype.trimStart
func stringTrimStartImpl(args []vm.Value) vm.Value {
	if len(args) < 1 {
		return vm.NewString("")
	}

	str := args[0].ToString()
	return vm.NewString(strings.TrimLeftFunc(str, unicode.IsSpace))
}

// stringTrimEndImpl implements String.prototype.trimEnd
func stringTrimEndImpl(args []vm.Value) vm.Value {
	if len(args) < 1 {
		return vm.NewString("")
	}

	str := args[0].ToString()
	return vm.NewString(strings.TrimRightFunc(str, unicode.IsSpace))
}

// stringRepeatImpl implements String.prototype.repeat
func stringRepeatImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.NewString("")
	}

	str := args[0].ToString()
	count := int(args[1].ToFloat())

	if count < 0 {
		return vm.NewString("") // In JS this would throw RangeError, but we'll be lenient
	}
	if count == 0 {
		return vm.NewString("")
	}

	return vm.NewString(strings.Repeat(str, count))
}

// stringLastIndexOfImpl implements String.prototype.lastIndexOf
func stringLastIndexOfImpl(args []vm.Value) vm.Value {
	if len(args) < 2 {
		return vm.Number(-1)
	}

	str := args[0].ToString()
	searchStr := args[1].ToString()

	// Find last occurrence
	lastIndex := -1
	searchLen := len(searchStr)
	if searchLen == 0 {
		return vm.Number(float64(len(str))) // Empty string found at end
	}

	for i := len(str) - searchLen; i >= 0; i-- {
		if str[i:i+searchLen] == searchStr {
			lastIndex = i
			break
		}
	}

	return vm.Number(float64(lastIndex))
}

// stringConcatImpl implements String.prototype.concat
func stringConcatImpl(args []vm.Value) vm.Value {
	if len(args) < 1 {
		return vm.NewString("")
	}

	str := args[0].ToString()
	var result strings.Builder
	result.WriteString(str)

	// Concatenate all additional arguments
	for i := 1; i < len(args); i++ {
		result.WriteString(args[i].ToString())
	}

	return vm.NewString(result.String())
}

// stringSplitImpl implements String.prototype.split
func stringSplitImpl(args []vm.Value) vm.Value {
	if len(args) < 1 {
		return vm.NewArray()
	}

	str := args[0].ToString()

	// If no separator provided, return array with the whole string
	if len(args) < 2 || args[1].Type() == vm.TypeUndefined {
		result := vm.NewArray()
		result.AsArray().Append(vm.NewString(str))
		return result
	}

	separator := args[1].ToString()

	// Handle empty separator - split into individual characters
	if separator == "" {
		result := vm.NewArray()
		for _, char := range str {
			result.AsArray().Append(vm.NewString(string(char)))
		}
		return result
	}

	// Normal split
	parts := strings.Split(str, separator)
	result := vm.NewArray()
	for _, part := range parts {
		result.AsArray().Append(vm.NewString(part))
	}

	return result
}

// stringConstructor implements the String() constructor
func stringConstructor(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.NewString("")
	}
	return vm.NewString(args[0].ToString())
}

// stringFromCharCode implements String.fromCharCode (static method)
func stringFromCharCode(args []vm.Value) vm.Value {
	if len(args) == 0 {
		return vm.NewString("")
	}

	result := make([]byte, len(args))
	for i, arg := range args {
		code := int(arg.ToFloat()) & 0xFFFF // Mask to 16 bits like JS
		result[i] = byte(code)
	}

	return vm.NewString(string(result))
}

// setupStringPrototype sets up String prototype methods for a specific VM instance  
// This ensures string methods are available on the VM-specific prototype
func setupStringPrototype(vmInstance *vm.VM) {
	stringProto := vmInstance.StringPrototype.AsPlainObject()
	
	// Register all string prototype methods to the VM-specific prototype
	stringProto.SetOwn("charAt", vm.NewNativeFunction(1, false, "charAt", stringCharAtImpl))
	stringProto.SetOwn("charCodeAt", vm.NewNativeFunction(1, false, "charCodeAt", stringCharCodeAtImpl))
	stringProto.SetOwn("substring", vm.NewNativeFunction(2, false, "substring", stringSubstringImpl))
	stringProto.SetOwn("slice", vm.NewNativeFunction(2, false, "slice", stringSliceImpl))
	stringProto.SetOwn("indexOf", vm.NewNativeFunction(1, false, "indexOf", stringIndexOfImpl))
	stringProto.SetOwn("includes", vm.NewNativeFunction(1, false, "includes", stringIncludesImpl))
	stringProto.SetOwn("startsWith", vm.NewNativeFunction(1, false, "startsWith", stringStartsWithImpl))
	stringProto.SetOwn("endsWith", vm.NewNativeFunction(1, false, "endsWith", stringEndsWithImpl))
	stringProto.SetOwn("toLowerCase", vm.NewNativeFunction(0, false, "toLowerCase", stringToLowerCaseImpl))
	stringProto.SetOwn("toUpperCase", vm.NewNativeFunction(0, false, "toUpperCase", stringToUpperCaseImpl))
	stringProto.SetOwn("trim", vm.NewNativeFunction(0, false, "trim", stringTrimImpl))
	stringProto.SetOwn("trimStart", vm.NewNativeFunction(0, false, "trimStart", stringTrimStartImpl))
	stringProto.SetOwn("trimEnd", vm.NewNativeFunction(0, false, "trimEnd", stringTrimEndImpl))
	stringProto.SetOwn("repeat", vm.NewNativeFunction(1, false, "repeat", stringRepeatImpl))
	stringProto.SetOwn("lastIndexOf", vm.NewNativeFunction(1, false, "lastIndexOf", stringLastIndexOfImpl))
	stringProto.SetOwn("concat", vm.NewNativeFunction(-1, true, "concat", stringConcatImpl))
	stringProto.SetOwn("split", vm.NewNativeFunction(1, false, "split", stringSplitImpl))
}

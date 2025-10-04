package builtins

import (
	"fmt"
	"paserati/pkg/types"
	"paserati/pkg/vm"
	"strings"
	"unicode"
)

type StringInitializer struct{}

func (s *StringInitializer) Name() string {
	return "String"
}

func (s *StringInitializer) Priority() int {
	return 300 // After Object (100) and Function (200)
}

func (s *StringInitializer) InitTypes(ctx *TypeContext) error {
	// Create String constructor type first (needed for constructor property)
	stringCtorType := types.NewSimpleFunction([]types.Type{types.Any}, types.String).
		WithProperty("fromCharCode", types.NewVariadicFunction([]types.Type{}, types.String, &types.ArrayType{ElementType: types.Number}))

	// Create String.prototype type with all methods
	// Note: 'this' is implicit and not included in type signatures
	stringProtoType := types.NewObjectType().
		WithProperty("charAt", types.NewSimpleFunction([]types.Type{types.Number}, types.String)).
		WithProperty("charCodeAt", types.NewSimpleFunction([]types.Type{types.Number}, types.Number)).
		WithProperty("slice", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.String, []bool{false, true})).
		WithProperty("substring", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.String, []bool{false, true})).
		WithProperty("substr", types.NewOptionalFunction([]types.Type{types.Number, types.Number}, types.String, []bool{false, true})).
		WithProperty("indexOf", types.NewOptionalFunction([]types.Type{types.String, types.Number}, types.Number, []bool{false, true})).
		WithProperty("lastIndexOf", types.NewOptionalFunction([]types.Type{types.String, types.Number}, types.Number, []bool{false, true})).
		WithProperty("includes", types.NewOptionalFunction([]types.Type{types.String, types.Number}, types.Boolean, []bool{false, true})).
		WithProperty("startsWith", types.NewOptionalFunction([]types.Type{types.String, types.Number}, types.Boolean, []bool{false, true})).
		WithProperty("endsWith", types.NewOptionalFunction([]types.Type{types.String, types.Number}, types.Boolean, []bool{false, true})).
		WithProperty("toLowerCase", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("toUpperCase", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("trim", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("trimStart", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("trimEnd", types.NewSimpleFunction([]types.Type{}, types.String)).
		WithProperty("repeat", types.NewSimpleFunction([]types.Type{types.Number}, types.String)).
		WithProperty("concat", types.NewVariadicFunction([]types.Type{}, types.String, types.String)).
		WithProperty("split", types.NewOptionalFunction([]types.Type{types.NewUnionType(types.String, types.RegExp), types.Number}, &types.ArrayType{ElementType: types.String}, []bool{false, true})).
		WithProperty("replace", types.NewSimpleFunction([]types.Type{types.NewUnionType(types.String, types.RegExp), types.String}, types.String)).
		WithProperty("match", types.NewSimpleFunction([]types.Type{types.NewUnionType(types.String, types.RegExp)}, types.NewUnionType(&types.ArrayType{ElementType: types.String}, types.Null))).
		WithProperty("search", types.NewSimpleFunction([]types.Type{types.NewUnionType(types.String, types.RegExp)}, types.Number)).
		WithProperty("constructor", types.Any) // Avoid circular reference, use Any for constructor property

	// Add Symbol.iterator to string prototype type to make strings iterable
	// Get the Iterator<T> generic type if available
	if iteratorType, found := ctx.GetType("Iterator"); found {
		if iteratorGeneric, ok := iteratorType.(*types.GenericType); ok {
			// Create Iterator<string> type for strings
			iteratorOfString := &types.InstantiatedType{
				Generic:       iteratorGeneric,
				TypeArguments: []types.Type{types.String},
			}
			// Add [Symbol.iterator](): Iterator<string> method (computed symbol key in types)
			stringProtoType = stringProtoType.WithProperty("__COMPUTED_PROPERTY__",
				types.NewSimpleFunction([]types.Type{}, iteratorOfString.Substitute()))
		}
	}

	// Register string primitive prototype
	ctx.SetPrimitivePrototype("string", stringProtoType)

	// Add prototype property to constructor
	stringCtorType = stringCtorType.WithProperty("prototype", stringProtoType)

	// Define String constructor in global environment
	return ctx.DefineGlobal("String", stringCtorType)
}

func (s *StringInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create String.prototype inheriting from Object.prototype
	stringProto := vm.NewObject(objectProto).AsPlainObject()

	// Add String prototype methods
	stringProto.SetOwn("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis()

		// If this is a primitive string, return it
		if thisStr.Type() == vm.TypeString {
			return thisStr, nil
		}

		// If this is a String wrapper object, extract [[PrimitiveValue]]
		if thisStr.IsObject() {
			if primitiveVal, exists := thisStr.AsPlainObject().GetOwn("[[PrimitiveValue]]"); exists {
				return primitiveVal, nil
			}
		}

		// TypeError: String.prototype.valueOf requires that 'this' be a String
		return vm.Undefined, fmt.Errorf("String.prototype.valueOf requires that 'this' be a String")
	}))

	stringProto.SetOwn("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis()

		// If this is a primitive string, return it
		if thisStr.Type() == vm.TypeString {
			return thisStr, nil
		}

		// If this is a String wrapper object, extract [[PrimitiveValue]]
		if thisStr.IsObject() {
			if primitiveVal, exists := thisStr.AsPlainObject().GetOwn("[[PrimitiveValue]]"); exists {
				if primitiveVal.Type() == vm.TypeString {
					return primitiveVal, nil
				}
			}
		}

		// TypeError: String.prototype.toString requires that 'this' be a String
		return vm.Undefined, fmt.Errorf("String.prototype.toString requires that 'this' be a String")
	}))

	stringProto.SetOwn("charAt", vm.NewNativeFunction(1, false, "charAt", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis().ToString()
		if len(args) < 1 {
			return vm.NewString(""), nil
		}
		index := int(args[0].ToFloat())
		if index < 0 || index >= len(thisStr) {
			return vm.NewString(""), nil
		}
		return vm.NewString(string(thisStr[index])), nil
	}))

	stringProto.SetOwn("charCodeAt", vm.NewNativeFunction(1, false, "charCodeAt", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis().ToString()
		if len(args) < 1 {
			return vm.NumberValue(float64(0x7FFFFFFF)), nil // NaN equivalent
		}
		index := int(args[0].ToFloat())
		if index < 0 || index >= len(thisStr) {
			return vm.NumberValue(float64(0x7FFFFFFF)), nil // NaN equivalent
		}
		return vm.NumberValue(float64(thisStr[index])), nil
	}))

	stringProto.SetOwn("slice", vm.NewNativeFunction(2, false, "slice", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis().ToString()
		length := len(thisStr)
		if len(args) < 1 {
			return vm.NewString(thisStr), nil
		}
		start := int(args[0].ToFloat())
		if start < 0 {
			start = length + start
			if start < 0 {
				start = 0
			}
		} else if start > length {
			start = length
		}
		end := length
		if len(args) >= 2 {
			end = int(args[1].ToFloat())
			if end < 0 {
				end = length + end
				if end < 0 {
					end = 0
				}
			} else if end > length {
				end = length
			}
		}
		if start >= end {
			return vm.NewString(""), nil
		}
		return vm.NewString(thisStr[start:end]), nil
	}))

	stringProto.SetOwn("substring", vm.NewNativeFunction(2, false, "substring", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis().ToString()
		length := len(thisStr)
		if len(args) < 1 {
			return vm.NewString(thisStr), nil
		}
		start := int(args[0].ToFloat())
		if start < 0 {
			start = 0
		} else if start > length {
			start = length
		}
		end := length
		if len(args) >= 2 {
			end = int(args[1].ToFloat())
			if end < 0 {
				end = 0
			} else if end > length {
				end = length
			}
		}
		// substring swaps start and end if start > end
		if start > end {
			start, end = end, start
		}
		return vm.NewString(thisStr[start:end]), nil
	}))

	stringProto.SetOwn("substr", vm.NewNativeFunction(2, false, "substr", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis().ToString()
		length := len(thisStr)
		if len(args) < 1 {
			return vm.NewString(thisStr), nil
		}
		start := int(args[0].ToFloat())
		if start < 0 {
			start = length + start
			if start < 0 {
				start = 0
			}
		} else if start >= length {
			return vm.NewString(""), nil
		}
		substrLength := length - start
		if len(args) >= 2 {
			substrLength = int(args[1].ToFloat())
			if substrLength < 0 {
				return vm.NewString(""), nil
			}
		}
		end := start + substrLength
		if end > length {
			end = length
		}
		return vm.NewString(thisStr[start:end]), nil
	}))

	stringProto.SetOwn("indexOf", vm.NewNativeFunction(2, false, "indexOf", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis().ToString()
		if len(args) < 1 {
			return vm.NumberValue(-1), nil
		}
		searchStr := args[0].ToString()
		position := 0
		if len(args) >= 2 {
			position = int(args[1].ToFloat())
			if position < 0 {
				position = 0
			}
		}
		if position >= len(thisStr) {
			return vm.NumberValue(-1), nil
		}
		index := strings.Index(thisStr[position:], searchStr)
		if index == -1 {
			return vm.NumberValue(-1), nil
		}
		return vm.NumberValue(float64(position + index)), nil
	}))

	stringProto.SetOwn("lastIndexOf", vm.NewNativeFunction(2, false, "lastIndexOf", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis().ToString()
		if len(args) < 1 {
			return vm.NumberValue(-1), nil
		}
		searchStr := args[0].ToString()
		position := len(thisStr)
		if len(args) >= 2 {
			position = int(args[1].ToFloat())
			if position < 0 {
				position = 0
			} else if position > len(thisStr) {
				position = len(thisStr)
			}
		}
		index := strings.LastIndex(thisStr[:position+len(searchStr)], searchStr)
		return vm.NumberValue(float64(index)), nil
	}))

	stringProto.SetOwn("includes", vm.NewNativeFunction(2, false, "includes", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis().ToString()
		if len(args) < 1 {
			return vm.BooleanValue(false), nil
		}
		searchStr := args[0].ToString()
		position := 0
		if len(args) >= 2 {
			position = int(args[1].ToFloat())
			if position < 0 {
				position = 0
			}
		}
		if position >= len(thisStr) {
			return vm.BooleanValue(false), nil
		}
		return vm.BooleanValue(strings.Contains(thisStr[position:], searchStr)), nil
	}))

	stringProto.SetOwn("startsWith", vm.NewNativeFunction(2, false, "startsWith", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis().ToString()
		if len(args) < 1 {
			return vm.BooleanValue(false), nil
		}
		searchStr := args[0].ToString()
		position := 0
		if len(args) >= 2 {
			position = int(args[1].ToFloat())
			if position < 0 {
				position = 0
			}
		}
		if position >= len(thisStr) {
			return vm.BooleanValue(false), nil
		}
		return vm.BooleanValue(strings.HasPrefix(thisStr[position:], searchStr)), nil
	}))

	stringProto.SetOwn("endsWith", vm.NewNativeFunction(2, false, "endsWith", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis().ToString()
		if len(args) < 1 {
			return vm.BooleanValue(false), nil
		}
		searchStr := args[0].ToString()
		length := len(thisStr)
		if len(args) >= 2 {
			length = int(args[1].ToFloat())
			if length < 0 {
				length = 0
			} else if length > len(thisStr) {
				length = len(thisStr)
			}
		}
		if length < len(searchStr) {
			return vm.BooleanValue(false), nil
		}
		return vm.BooleanValue(strings.HasSuffix(thisStr[:length], searchStr)), nil
	}))

	stringProto.SetOwn("toLowerCase", vm.NewNativeFunction(0, false, "toLowerCase", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis().ToString()
		return vm.NewString(strings.ToLower(thisStr)), nil
	}))

	stringProto.SetOwn("toUpperCase", vm.NewNativeFunction(0, false, "toUpperCase", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis().ToString()
		return vm.NewString(strings.ToUpper(thisStr)), nil
	}))

	stringProto.SetOwn("trim", vm.NewNativeFunction(0, false, "trim", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis().ToString()
		return vm.NewString(strings.TrimSpace(thisStr)), nil
	}))

	stringProto.SetOwn("trimStart", vm.NewNativeFunction(0, false, "trimStart", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis().ToString()
		return vm.NewString(strings.TrimLeftFunc(thisStr, unicode.IsSpace)), nil
	}))

	stringProto.SetOwn("trimEnd", vm.NewNativeFunction(0, false, "trimEnd", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis().ToString()
		return vm.NewString(strings.TrimRightFunc(thisStr, unicode.IsSpace)), nil
	}))

	stringProto.SetOwn("repeat", vm.NewNativeFunction(1, false, "repeat", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis().ToString()
		if len(args) < 1 {
			return vm.NewString(""), nil
		}
		count := int(args[0].ToFloat())
		if count < 0 {
			// TODO: Should throw RangeError
			return vm.NewString(""), nil
		}
		if count == 0 || thisStr == "" {
			return vm.NewString(""), nil
		}
		return vm.NewString(strings.Repeat(thisStr, count)), nil
	}))

	stringProto.SetOwn("concat", vm.NewNativeFunction(0, true, "concat", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis().ToString()
		result := thisStr
		for i := 0; i < len(args); i++ {
			result += args[i].ToString()
		}
		return vm.NewString(result), nil
	}))

	stringProto.SetOwn("split", vm.NewNativeFunction(2, false, "split", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis().ToString()
		if len(args) == 0 {
			// No separator - return array with whole string
			return vm.NewArrayWithArgs([]vm.Value{vm.NewString(thisStr)}), nil
		}

		separatorArg := args[0]
		limit := -1
		if len(args) >= 2 {
			limitVal := args[1].ToFloat()
			if limitVal > 0 {
				limit = int(limitVal)
			} else if limitVal <= 0 {
				return vm.NewArray(), nil
			}
		}

		if separatorArg.IsRegExp() {
			// RegExp separator
			regex := separatorArg.AsRegExpObject()
			compiledRegex := regex.GetCompiledRegex()

			parts := compiledRegex.Split(thisStr, -1)
			if limit > 0 && len(parts) > limit {
				parts = parts[:limit]
			}
			elements := make([]vm.Value, len(parts))
			for i, part := range parts {
				elements[i] = vm.NewString(part)
			}
			return vm.NewArrayWithArgs(elements), nil
		} else {
			// String separator
			separator := separatorArg.ToString()
			if separator == "" {
				// Split into individual characters
				runes := []rune(thisStr)
				count := len(runes)
				if limit > 0 && limit < count {
					count = limit
				}
				elements := make([]vm.Value, count)
				for i := 0; i < count; i++ {
					elements[i] = vm.NewString(string(runes[i]))
				}
				return vm.NewArrayWithArgs(elements), nil
			}

			// Normal string split
			parts := strings.Split(thisStr, separator)
			if limit > 0 && len(parts) > limit {
				parts = parts[:limit]
			}
			elements := make([]vm.Value, len(parts))
			for i, part := range parts {
				elements[i] = vm.NewString(part)
			}
			return vm.NewArrayWithArgs(elements), nil
		}
	}))

	stringProto.SetOwn("replace", vm.NewNativeFunction(2, false, "replace", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis().ToString()
		if len(args) < 2 {
			return vm.NewString(thisStr), nil
		}

		searchArg := args[0]
		replaceValue := args[1].ToString()

		if searchArg.IsRegExp() {
			// RegExp argument
			regex := searchArg.AsRegExpObject()
			compiledRegex := regex.GetCompiledRegex()

			if regex.IsGlobal() {
				// Global replace: replace all matches
				result := compiledRegex.ReplaceAllString(thisStr, replaceValue)
				return vm.NewString(result), nil
			} else {
				// Non-global: replace first match only
				if loc := compiledRegex.FindStringIndex(thisStr); loc != nil {
					// Replace only the first match
					result := thisStr[:loc[0]] + replaceValue + thisStr[loc[1]:]
					return vm.NewString(result), nil
				}
				return vm.NewString(thisStr), nil
			}
		} else {
			// String argument - legacy behavior (replace first occurrence only)
			searchValue := searchArg.ToString()
			result := strings.Replace(thisStr, searchValue, replaceValue, 1)
			return vm.NewString(result), nil
		}
	}))

	stringProto.SetOwn("match", vm.NewNativeFunction(1, false, "match", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis().ToString()
		if len(args) < 1 {
			return vm.Null, nil
		}

		arg := args[0]
		if arg.IsRegExp() {
			// RegExp argument
			regex := arg.AsRegExpObject()
			compiledRegex := regex.GetCompiledRegex()

			if regex.IsGlobal() {
				// Global match: find all matches
				matches := compiledRegex.FindAllString(thisStr, -1)
				if len(matches) == 0 {
					return vm.Null, nil
				}
				result := vm.NewArray()
				arr := result.AsArray()
				for _, match := range matches {
					arr.Append(vm.NewString(match))
				}
				return result, nil
			} else {
				// Non-global: find first match with capture groups
				matches := compiledRegex.FindStringSubmatch(thisStr)
				if matches == nil {
					return vm.Null, nil
				}
				result := vm.NewArray()
				arr := result.AsArray()
				for _, match := range matches {
					arr.Append(vm.NewString(match))
				}
				return result, nil
			}
		} else {
			// String argument - legacy behavior
			pattern := arg.ToString()
			if strings.Contains(thisStr, pattern) {
				return vm.NewArrayWithArgs([]vm.Value{vm.NewString(pattern)}), nil
			}
			return vm.Null, nil
		}
	}))

	stringProto.SetOwn("search", vm.NewNativeFunction(1, false, "search", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis().ToString()
		if len(args) < 1 {
			return vm.NumberValue(-1), nil
		}

		arg := args[0]
		if arg.IsRegExp() {
			// RegExp argument
			regex := arg.AsRegExpObject()
			compiledRegex := regex.GetCompiledRegex()

			loc := compiledRegex.FindStringIndex(thisStr)
			if loc == nil {
				return vm.NumberValue(-1), nil
			}
			return vm.NumberValue(float64(loc[0])), nil
		} else {
			// String argument - legacy behavior
			searchValue := arg.ToString()
			index := strings.Index(thisStr, searchValue)
			return vm.NumberValue(float64(index)), nil
		}
	}))

	// Create String constructor
	stringCtor := vm.NewNativeFunction(-1, true, "String", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewString(""), nil
		}
		return vm.NewString(args[0].ToString()), nil
	})

	// Make it a proper constructor with static methods
	ctorWithProps := vm.NewNativeFunctionWithProps(-1, true, "String", func(args []vm.Value) (vm.Value, error) {
		// Determine the primitive string value
		var primitiveValue string
		if len(args) == 0 {
			primitiveValue = ""
		} else {
			primitiveValue = args[0].ToString()
		}

		// If called with 'new', return a String wrapper object
		if vmInstance.IsConstructorCall() {
			return vmInstance.NewStringObject(primitiveValue), nil
		}
		// Otherwise, return primitive string (type coercion)
		return vm.NewString(primitiveValue), nil
	})

	// Add prototype property
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("prototype", vm.NewValueFromPlainObject(stringProto))

	// Add static methods
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("fromCharCode", vm.NewNativeFunction(0, true, "fromCharCode", func(args []vm.Value) (vm.Value, error) {
		if len(args) == 0 {
			return vm.NewString(""), nil
		}
		result := make([]byte, len(args))
		for i, arg := range args {
			code := int(arg.ToFloat()) & 0xFFFF // Mask to 16 bits like JS
			result[i] = byte(code)
		}
		return vm.NewString(string(result)), nil
	}))

	stringCtor = ctorWithProps

	// Set constructor property on prototype
	stringProto.SetOwn("constructor", stringCtor)

	// Add Symbol.iterator implementation for strings (native symbol key)
	strIterFn := vm.NewNativeFunction(0, false, "[Symbol.iterator]", func(args []vm.Value) (vm.Value, error) {
		thisStr := vmInstance.GetThis().ToString()

		// Create a string iterator object
		return createStringIterator(vmInstance, thisStr), nil
	})
	stringProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolIterator), strIterFn, nil, nil, nil)

	// Set String prototype in VM
	vmInstance.StringPrototype = vm.NewValueFromPlainObject(stringProto)

	// Register String constructor as global
	return ctx.DefineGlobal("String", stringCtor)
}

// createStringIterator creates an iterator object for string iteration
func createStringIterator(vmInstance *vm.VM, str string) vm.Value {
	// Create iterator object inheriting from Object.prototype
	iterator := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

	// Iterator state: current index
	currentIndex := 0

	// Add next() method to iterator
	iterator.SetOwn("next", vm.NewNativeFunction(0, false, "next", func(args []vm.Value) (vm.Value, error) {
		// Create iterator result object {value, done}
		result := vm.NewObject(vmInstance.ObjectPrototype).AsPlainObject()

		if currentIndex >= len(str) {
			// Iterator is exhausted
			result.SetOwn("value", vm.Undefined)
			result.SetOwn("done", vm.BooleanValue(true))
		} else {
			// Return current character and advance
			char := string(str[currentIndex])
			result.SetOwn("value", vm.NewString(char))
			result.SetOwn("done", vm.BooleanValue(false))
			currentIndex++
		}

		return vm.NewValueFromPlainObject(result), nil
	}))

	return vm.NewValueFromPlainObject(iterator)
}

package builtins

import (
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
	stringProto.SetOwn("charAt", vm.NewNativeFunction(1, false, "charAt", func(args []vm.Value) vm.Value {
		thisStr := vmInstance.GetThis().ToString()
		if len(args) < 1 {
			return vm.NewString("")
		}
		index := int(args[0].ToFloat())
		if index < 0 || index >= len(thisStr) {
			return vm.NewString("")
		}
		return vm.NewString(string(thisStr[index]))
	}))

	stringProto.SetOwn("charCodeAt", vm.NewNativeFunction(1, false, "charCodeAt", func(args []vm.Value) vm.Value {
		thisStr := vmInstance.GetThis().ToString()
		if len(args) < 1 {
			return vm.NumberValue(float64(0x7FFFFFFF)) // NaN equivalent
		}
		index := int(args[0].ToFloat())
		if index < 0 || index >= len(thisStr) {
			return vm.NumberValue(float64(0x7FFFFFFF)) // NaN equivalent
		}
		return vm.NumberValue(float64(thisStr[index]))
	}))

	stringProto.SetOwn("slice", vm.NewNativeFunction(2, false, "slice", func(args []vm.Value) vm.Value {
		thisStr := vmInstance.GetThis().ToString()
		length := len(thisStr)
		if len(args) < 1 {
			return vm.NewString(thisStr)
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
			return vm.NewString("")
		}
		return vm.NewString(thisStr[start:end])
	}))

	stringProto.SetOwn("substring", vm.NewNativeFunction(2, false, "substring", func(args []vm.Value) vm.Value {
		thisStr := vmInstance.GetThis().ToString()
		length := len(thisStr)
		if len(args) < 1 {
			return vm.NewString(thisStr)
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
		return vm.NewString(thisStr[start:end])
	}))

	stringProto.SetOwn("substr", vm.NewNativeFunction(2, false, "substr", func(args []vm.Value) vm.Value {
		thisStr := vmInstance.GetThis().ToString()
		length := len(thisStr)
		if len(args) < 1 {
			return vm.NewString(thisStr)
		}
		start := int(args[0].ToFloat())
		if start < 0 {
			start = length + start
			if start < 0 {
				start = 0
			}
		} else if start >= length {
			return vm.NewString("")
		}
		substrLength := length - start
		if len(args) >= 2 {
			substrLength = int(args[1].ToFloat())
			if substrLength < 0 {
				return vm.NewString("")
			}
		}
		end := start + substrLength
		if end > length {
			end = length
		}
		return vm.NewString(thisStr[start:end])
	}))

	stringProto.SetOwn("indexOf", vm.NewNativeFunction(2, false, "indexOf", func(args []vm.Value) vm.Value {
		thisStr := vmInstance.GetThis().ToString()
		if len(args) < 1 {
			return vm.NumberValue(-1)
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
			return vm.NumberValue(-1)
		}
		index := strings.Index(thisStr[position:], searchStr)
		if index == -1 {
			return vm.NumberValue(-1)
		}
		return vm.NumberValue(float64(position + index))
	}))

	stringProto.SetOwn("lastIndexOf", vm.NewNativeFunction(2, false, "lastIndexOf", func(args []vm.Value) vm.Value {
		thisStr := vmInstance.GetThis().ToString()
		if len(args) < 1 {
			return vm.NumberValue(-1)
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
		return vm.NumberValue(float64(index))
	}))

	stringProto.SetOwn("includes", vm.NewNativeFunction(2, false, "includes", func(args []vm.Value) vm.Value {
		thisStr := vmInstance.GetThis().ToString()
		if len(args) < 1 {
			return vm.BooleanValue(false)
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
			return vm.BooleanValue(false)
		}
		return vm.BooleanValue(strings.Contains(thisStr[position:], searchStr))
	}))

	stringProto.SetOwn("startsWith", vm.NewNativeFunction(2, false, "startsWith", func(args []vm.Value) vm.Value {
		thisStr := vmInstance.GetThis().ToString()
		if len(args) < 1 {
			return vm.BooleanValue(false)
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
			return vm.BooleanValue(false)
		}
		return vm.BooleanValue(strings.HasPrefix(thisStr[position:], searchStr))
	}))

	stringProto.SetOwn("endsWith", vm.NewNativeFunction(2, false, "endsWith", func(args []vm.Value) vm.Value {
		thisStr := vmInstance.GetThis().ToString()
		if len(args) < 1 {
			return vm.BooleanValue(false)
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
			return vm.BooleanValue(false)
		}
		return vm.BooleanValue(strings.HasSuffix(thisStr[:length], searchStr))
	}))

	stringProto.SetOwn("toLowerCase", vm.NewNativeFunction(0, false, "toLowerCase", func(args []vm.Value) vm.Value {
		thisStr := vmInstance.GetThis().ToString()
		return vm.NewString(strings.ToLower(thisStr))
	}))

	stringProto.SetOwn("toUpperCase", vm.NewNativeFunction(0, false, "toUpperCase", func(args []vm.Value) vm.Value {
		thisStr := vmInstance.GetThis().ToString()
		return vm.NewString(strings.ToUpper(thisStr))
	}))

	stringProto.SetOwn("trim", vm.NewNativeFunction(0, false, "trim", func(args []vm.Value) vm.Value {
		thisStr := vmInstance.GetThis().ToString()
		return vm.NewString(strings.TrimSpace(thisStr))
	}))

	stringProto.SetOwn("trimStart", vm.NewNativeFunction(0, false, "trimStart", func(args []vm.Value) vm.Value {
		thisStr := vmInstance.GetThis().ToString()
		return vm.NewString(strings.TrimLeftFunc(thisStr, unicode.IsSpace))
	}))

	stringProto.SetOwn("trimEnd", vm.NewNativeFunction(0, false, "trimEnd", func(args []vm.Value) vm.Value {
		thisStr := vmInstance.GetThis().ToString()
		return vm.NewString(strings.TrimRightFunc(thisStr, unicode.IsSpace))
	}))

	stringProto.SetOwn("repeat", vm.NewNativeFunction(1, false, "repeat", func(args []vm.Value) vm.Value {
		thisStr := vmInstance.GetThis().ToString()
		if len(args) < 1 {
			return vm.NewString("")
		}
		count := int(args[0].ToFloat())
		if count < 0 {
			// TODO: Should throw RangeError
			return vm.NewString("")
		}
		if count == 0 || thisStr == "" {
			return vm.NewString("")
		}
		return vm.NewString(strings.Repeat(thisStr, count))
	}))

	stringProto.SetOwn("concat", vm.NewNativeFunction(0, true, "concat", func(args []vm.Value) vm.Value {
		thisStr := vmInstance.GetThis().ToString()
		result := thisStr
		for i := 0; i < len(args); i++ {
			result += args[i].ToString()
		}
		return vm.NewString(result)
	}))

	stringProto.SetOwn("split", vm.NewNativeFunction(2, false, "split", func(args []vm.Value) vm.Value {
		thisStr := vmInstance.GetThis().ToString()
		if len(args) == 0 {
			// No separator - return array with whole string
			return vm.NewArrayWithArgs([]vm.Value{vm.NewString(thisStr)})
		}
		
		separatorArg := args[0]
		limit := -1
		if len(args) >= 2 {
			limitVal := args[1].ToFloat()
			if limitVal > 0 {
				limit = int(limitVal)
			} else if limitVal <= 0 {
				return vm.NewArray()
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
			return vm.NewArrayWithArgs(elements)
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
				return vm.NewArrayWithArgs(elements)
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
			return vm.NewArrayWithArgs(elements)
		}
	}))

	stringProto.SetOwn("replace", vm.NewNativeFunction(2, false, "replace", func(args []vm.Value) vm.Value {
		thisStr := vmInstance.GetThis().ToString()
		if len(args) < 2 {
			return vm.NewString(thisStr)
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
				return vm.NewString(result)
			} else {
				// Non-global: replace first match only
				if loc := compiledRegex.FindStringIndex(thisStr); loc != nil {
					// Replace only the first match
					result := thisStr[:loc[0]] + replaceValue + thisStr[loc[1]:]
					return vm.NewString(result)
				}
				return vm.NewString(thisStr)
			}
		} else {
			// String argument - legacy behavior (replace first occurrence only)
			searchValue := searchArg.ToString()
			result := strings.Replace(thisStr, searchValue, replaceValue, 1)
			return vm.NewString(result)
		}
	}))

	stringProto.SetOwn("match", vm.NewNativeFunction(1, false, "match", func(args []vm.Value) vm.Value {
		thisStr := vmInstance.GetThis().ToString()
		if len(args) < 1 {
			return vm.Null
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
					return vm.Null
				}
				result := vm.NewArray()
				arr := result.AsArray()
				for _, match := range matches {
					arr.Append(vm.NewString(match))
				}
				return result
			} else {
				// Non-global: find first match with capture groups
				matches := compiledRegex.FindStringSubmatch(thisStr)
				if matches == nil {
					return vm.Null
				}
				result := vm.NewArray()
				arr := result.AsArray()
				for _, match := range matches {
					arr.Append(vm.NewString(match))
				}
				return result
			}
		} else {
			// String argument - legacy behavior
			pattern := arg.ToString()
			if strings.Contains(thisStr, pattern) {
				return vm.NewArrayWithArgs([]vm.Value{vm.NewString(pattern)})
			}
			return vm.Null
		}
	}))

	stringProto.SetOwn("search", vm.NewNativeFunction(1, false, "search", func(args []vm.Value) vm.Value {
		thisStr := vmInstance.GetThis().ToString()
		if len(args) < 1 {
			return vm.NumberValue(-1)
		}
		
		arg := args[0]
		if arg.IsRegExp() {
			// RegExp argument
			regex := arg.AsRegExpObject()
			compiledRegex := regex.GetCompiledRegex()
			
			loc := compiledRegex.FindStringIndex(thisStr)
			if loc == nil {
				return vm.NumberValue(-1)
			}
			return vm.NumberValue(float64(loc[0]))
		} else {
			// String argument - legacy behavior
			searchValue := arg.ToString()
			index := strings.Index(thisStr, searchValue)
			return vm.NumberValue(float64(index))
		}
	}))

	// Create String constructor
	stringCtor := vm.NewNativeFunction(-1, true, "String", func(args []vm.Value) vm.Value {
		if len(args) == 0 {
			return vm.NewString("")
		}
		return vm.NewString(args[0].ToString())
	})

	// Make it a proper constructor with static methods
	ctorWithProps := vm.NewNativeFunctionWithProps(-1, true, "String", func(args []vm.Value) vm.Value {
		if len(args) == 0 {
			return vm.NewString("")
		}
		return vm.NewString(args[0].ToString())
	})

	// Add prototype property
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("prototype", vm.NewValueFromPlainObject(stringProto))

	// Add static methods
	ctorWithProps.AsNativeFunctionWithProps().Properties.SetOwn("fromCharCode", vm.NewNativeFunction(0, true, "fromCharCode", func(args []vm.Value) vm.Value {
		if len(args) == 0 {
			return vm.NewString("")
		}
		result := make([]byte, len(args))
		for i, arg := range args {
			code := int(arg.ToFloat()) & 0xFFFF // Mask to 16 bits like JS
			result[i] = byte(code)
		}
		return vm.NewString(string(result))
	}))

	stringCtor = ctorWithProps

	// Set constructor property on prototype
	stringProto.SetOwn("constructor", stringCtor)

	// Set String prototype in VM
	vmInstance.StringPrototype = vm.NewValueFromPlainObject(stringProto)

	// Register String constructor as global
	return ctx.DefineGlobal("String", stringCtor)
}

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
		WithProperty("split", types.NewOptionalFunction([]types.Type{types.String, types.Number}, &types.ArrayType{ElementType: types.String}, []bool{false, true})).
		WithProperty("replace", types.NewSimpleFunction([]types.Type{types.String, types.String}, types.String)).
		WithProperty("match", types.NewSimpleFunction([]types.Type{types.String}, types.NewUnionType(&types.ArrayType{ElementType: types.String}, types.Null))).
		WithProperty("search", types.NewSimpleFunction([]types.Type{types.String}, types.Number))

	// Register string primitive prototype
	ctx.SetPrimitivePrototype("string", stringProtoType)

	// Create String constructor type
	stringCtorType := types.NewSimpleFunction([]types.Type{types.Any}, types.String).
		WithProperty("fromCharCode", types.NewVariadicFunction([]types.Type{}, types.String, types.Number)).
		WithProperty("prototype", stringProtoType)

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
		thisStr := args[0].ToString()
		if len(args) < 2 {
			return vm.NewString("")
		}
		index := int(args[1].ToFloat())
		if index < 0 || index >= len(thisStr) {
			return vm.NewString("")
		}
		return vm.NewString(string(thisStr[index]))
	}))

	stringProto.SetOwn("charCodeAt", vm.NewNativeFunction(1, false, "charCodeAt", func(args []vm.Value) vm.Value {
		thisStr := args[0].ToString()
		if len(args) < 2 {
			return vm.NumberValue(float64(0x7FFFFFFF)) // NaN equivalent
		}
		index := int(args[1].ToFloat())
		if index < 0 || index >= len(thisStr) {
			return vm.NumberValue(float64(0x7FFFFFFF)) // NaN equivalent
		}
		return vm.NumberValue(float64(thisStr[index]))
	}))

	stringProto.SetOwn("slice", vm.NewNativeFunction(2, false, "slice", func(args []vm.Value) vm.Value {
		thisStr := args[0].ToString()
		length := len(thisStr)
		if len(args) < 2 {
			return vm.NewString(thisStr)
		}
		start := int(args[1].ToFloat())
		if start < 0 {
			start = length + start
			if start < 0 {
				start = 0
			}
		} else if start > length {
			start = length
		}
		end := length
		if len(args) >= 3 {
			end = int(args[2].ToFloat())
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
		thisStr := args[0].ToString()
		length := len(thisStr)
		if len(args) < 2 {
			return vm.NewString(thisStr)
		}
		start := int(args[1].ToFloat())
		if start < 0 {
			start = 0
		} else if start > length {
			start = length
		}
		end := length
		if len(args) >= 3 {
			end = int(args[2].ToFloat())
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
		thisStr := args[0].ToString()
		length := len(thisStr)
		if len(args) < 2 {
			return vm.NewString(thisStr)
		}
		start := int(args[1].ToFloat())
		if start < 0 {
			start = length + start
			if start < 0 {
				start = 0
			}
		} else if start >= length {
			return vm.NewString("")
		}
		substrLength := length - start
		if len(args) >= 3 {
			substrLength = int(args[2].ToFloat())
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
		thisStr := args[0].ToString()
		if len(args) < 2 {
			return vm.NumberValue(-1)
		}
		searchStr := args[1].ToString()
		position := 0
		if len(args) >= 3 {
			position = int(args[2].ToFloat())
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
		thisStr := args[0].ToString()
		if len(args) < 2 {
			return vm.NumberValue(-1)
		}
		searchStr := args[1].ToString()
		position := len(thisStr)
		if len(args) >= 3 {
			position = int(args[2].ToFloat())
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
		thisStr := args[0].ToString()
		if len(args) < 2 {
			return vm.BooleanValue(false)
		}
		searchStr := args[1].ToString()
		position := 0
		if len(args) >= 3 {
			position = int(args[2].ToFloat())
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
		thisStr := args[0].ToString()
		if len(args) < 2 {
			return vm.BooleanValue(false)
		}
		searchStr := args[1].ToString()
		position := 0
		if len(args) >= 3 {
			position = int(args[2].ToFloat())
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
		thisStr := args[0].ToString()
		if len(args) < 2 {
			return vm.BooleanValue(false)
		}
		searchStr := args[1].ToString()
		length := len(thisStr)
		if len(args) >= 3 {
			length = int(args[2].ToFloat())
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
		thisStr := args[0].ToString()
		return vm.NewString(strings.ToLower(thisStr))
	}))

	stringProto.SetOwn("toUpperCase", vm.NewNativeFunction(0, false, "toUpperCase", func(args []vm.Value) vm.Value {
		thisStr := args[0].ToString()
		return vm.NewString(strings.ToUpper(thisStr))
	}))

	stringProto.SetOwn("trim", vm.NewNativeFunction(0, false, "trim", func(args []vm.Value) vm.Value {
		thisStr := args[0].ToString()
		return vm.NewString(strings.TrimSpace(thisStr))
	}))

	stringProto.SetOwn("trimStart", vm.NewNativeFunction(0, false, "trimStart", func(args []vm.Value) vm.Value {
		thisStr := args[0].ToString()
		return vm.NewString(strings.TrimLeftFunc(thisStr, unicode.IsSpace))
	}))

	stringProto.SetOwn("trimEnd", vm.NewNativeFunction(0, false, "trimEnd", func(args []vm.Value) vm.Value {
		thisStr := args[0].ToString()
		return vm.NewString(strings.TrimRightFunc(thisStr, unicode.IsSpace))
	}))

	stringProto.SetOwn("repeat", vm.NewNativeFunction(1, false, "repeat", func(args []vm.Value) vm.Value {
		thisStr := args[0].ToString()
		if len(args) < 2 {
			return vm.NewString("")
		}
		count := int(args[1].ToFloat())
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
		thisStr := args[0].ToString()
		result := thisStr
		for i := 1; i < len(args); i++ {
			result += args[i].ToString()
		}
		return vm.NewString(result)
	}))

	stringProto.SetOwn("split", vm.NewNativeFunction(2, false, "split", func(args []vm.Value) vm.Value {
		thisStr := args[0].ToString()
		if len(args) < 2 {
			// No separator - return array with whole string
			return vm.NewArrayWithArgs([]vm.Value{vm.NewString(thisStr)})
		}
		separator := args[1].ToString()
		limit := -1
		if len(args) >= 3 {
			limit = int(args[2].ToFloat())
			if limit <= 0 {
				return vm.NewArray()
			}
		}
		if separator == "" {
			// Split into individual characters
			runes := []rune(thisStr)
			elements := make([]vm.Value, 0, len(runes))
			for i, r := range runes {
				if limit > 0 && i >= limit {
					break
				}
				elements = append(elements, vm.NewString(string(r)))
			}
			return vm.NewArrayWithArgs(elements)
		}
		parts := strings.Split(thisStr, separator)
		if limit > 0 && len(parts) > limit {
			parts = parts[:limit]
		}
		elements := make([]vm.Value, len(parts))
		for i, part := range parts {
			elements[i] = vm.NewString(part)
		}
		return vm.NewArrayWithArgs(elements)
	}))

	stringProto.SetOwn("replace", vm.NewNativeFunction(2, false, "replace", func(args []vm.Value) vm.Value {
		thisStr := args[0].ToString()
		if len(args) < 3 {
			return vm.NewString(thisStr)
		}
		searchValue := args[1].ToString()
		replaceValue := args[2].ToString()
		// Simple replace - only replaces first occurrence
		result := strings.Replace(thisStr, searchValue, replaceValue, 1)
		return vm.NewString(result)
	}))

	stringProto.SetOwn("match", vm.NewNativeFunction(1, false, "match", func(args []vm.Value) vm.Value {
		thisStr := args[0].ToString()
		if len(args) < 2 {
			return vm.Null
		}
		pattern := args[1].ToString()
		// Simple match - just check if pattern exists
		if strings.Contains(thisStr, pattern) {
			// Return array with match
			return vm.NewArrayWithArgs([]vm.Value{vm.NewString(pattern)})
		}
		return vm.Null
	}))

	stringProto.SetOwn("search", vm.NewNativeFunction(1, false, "search", func(args []vm.Value) vm.Value {
		thisStr := args[0].ToString()
		if len(args) < 2 {
			return vm.NumberValue(-1)
		}
		searchValue := args[1].ToString()
		// Simple search - just find first occurrence
		index := strings.Index(thisStr, searchValue)
		return vm.NumberValue(float64(index))
	}))

	// Create String constructor
	stringCtor := vm.NewNativeFunction(-1, true, "String", func(args []vm.Value) vm.Value {
		if len(args) == 0 {
			return vm.NewString("")
		}
		return vm.NewString(args[0].ToString())
	})

	// Make it a proper constructor with static methods
	if ctorObj := stringCtor.AsNativeFunction(); ctorObj != nil {
		// Convert to object with properties  
		ctorWithProps := vm.NewNativeFunctionWithProps(ctorObj.Arity, ctorObj.Variadic, ctorObj.Name, ctorObj.Fn)

		// Add prototype property
		// TODO: Uncomment when we can use Properties field
		// ctorWithProps.Properties.SetOwn("prototype", vm.NewValueFromPlainObject(stringProto))

		// Add static methods
		// TODO: Uncomment when we can use Properties field
		// ctorWithProps.Properties.SetOwn("fromCharCode", vm.NewNativeFunction(0, true, "fromCharCode", stringFromCharCodeImpl))

		stringCtor = ctorWithProps
	}

	// Set String prototype in VM
	vmInstance.StringPrototype = vm.NewValueFromPlainObject(stringProto)

	// Register String constructor as global
	return ctx.DefineGlobal("String", stringCtor)
}
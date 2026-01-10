package builtins

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// processReplacementPattern processes $ patterns in replacement strings
// $$ -> $
// $& -> matched substring
// $` -> portion before match
// $' -> portion after match
// $n -> nth capture group (1-9)
// $nn -> nth capture group (01-99)
func processReplacementPattern(str string, match []int, replacement string) string {
	var result strings.Builder
	i := 0
	for i < len(replacement) {
		if replacement[i] == '$' && i+1 < len(replacement) {
			switch replacement[i+1] {
			case '$':
				// $$ -> $
				result.WriteByte('$')
				i += 2
			case '&':
				// $& -> matched substring
				result.WriteString(str[match[0]:match[1]])
				i += 2
			case '`':
				// $` -> portion before match
				result.WriteString(str[:match[0]])
				i += 2
			case '\'':
				// $' -> portion after match
				result.WriteString(str[match[1]:])
				i += 2
			default:
				// Check for $n or $nn (capture group reference)
				if replacement[i+1] >= '0' && replacement[i+1] <= '9' {
					// Try to parse two digits first
					numStr := string(replacement[i+1])
					endIdx := i + 2
					if i+2 < len(replacement) && replacement[i+2] >= '0' && replacement[i+2] <= '9' {
						numStr += string(replacement[i+2])
						endIdx = i + 3
					}
					if num, err := strconv.Atoi(numStr); err == nil {
						// Check if we have enough capture groups
						groupIdx := num * 2
						if groupIdx < len(match) && groupIdx > 0 {
							if match[groupIdx] >= 0 && match[groupIdx+1] >= 0 {
								result.WriteString(str[match[groupIdx]:match[groupIdx+1]])
							}
							i = endIdx
							continue
						}
						// If two-digit didn't work, try one digit
						if len(numStr) == 2 {
							num, _ = strconv.Atoi(string(replacement[i+1]))
							groupIdx = num * 2
							if groupIdx < len(match) && groupIdx > 0 {
								if match[groupIdx] >= 0 && match[groupIdx+1] >= 0 {
									result.WriteString(str[match[groupIdx]:match[groupIdx+1]])
								}
								i += 2
								continue
							}
						}
					}
					// If no valid group, output literally
					result.WriteByte('$')
					i++
				} else {
					// Unknown $ sequence, output literally
					result.WriteByte('$')
					i++
				}
			}
		} else {
			result.WriteByte(replacement[i])
			i++
		}
	}
	return result.String()
}

type RegExpInitializer struct{}

func (r *RegExpInitializer) Name() string {
	return "RegExp"
}

func (r *RegExpInitializer) Priority() int {
	return PriorityRegExp // After basic types
}

func (r *RegExpInitializer) InitTypes(ctx *TypeContext) error {
	// Create RegExp.prototype type with methods
	regexpProtoType := types.NewObjectType().
		WithProperty("source", types.String).
		WithProperty("flags", types.String).
		WithProperty("global", types.Boolean).
		WithProperty("ignoreCase", types.Boolean).
		WithProperty("multiline", types.Boolean).
		WithProperty("dotAll", types.Boolean).
		WithProperty("lastIndex", types.Number).
		WithProperty("test", types.NewSimpleFunction([]types.Type{types.String}, types.Boolean)).
		WithProperty("exec", types.NewSimpleFunction([]types.Type{types.String}, types.NewUnionType(types.Null, &types.ArrayType{ElementType: types.String}))).
		WithProperty("toString", types.NewSimpleFunction([]types.Type{}, types.String))

	// Register RegExp primitive prototype
	ctx.SetPrimitivePrototype("RegExp", regexpProtoType)

	// Create RegExp constructor type
	regexpCtorType := types.NewObjectType().
		WithSimpleCallSignature([]types.Type{}, types.RegExp).                                // RegExp() -> RegExp
		WithSimpleCallSignature([]types.Type{types.String}, types.RegExp).                    // RegExp(pattern) -> RegExp
		WithSimpleCallSignature([]types.Type{types.String, types.String}, types.RegExp).      // RegExp(pattern, flags) -> RegExp
		WithSimpleCallSignature([]types.Type{types.RegExp}, types.RegExp).                    // RegExp(regexObj) -> RegExp
		WithSimpleConstructSignature([]types.Type{}, types.RegExp).                           // new RegExp() -> RegExp
		WithSimpleConstructSignature([]types.Type{types.String}, types.RegExp).               // new RegExp(pattern) -> RegExp
		WithSimpleConstructSignature([]types.Type{types.String, types.String}, types.RegExp). // new RegExp(pattern, flags) -> RegExp
		WithSimpleConstructSignature([]types.Type{types.RegExp}, types.RegExp).               // new RegExp(regexObj) -> RegExp
		WithProperty("prototype", regexpProtoType)

	return ctx.DefineGlobal("RegExp", regexpCtorType)
}

func (r *RegExpInitializer) InitRuntime(ctx *RuntimeContext) error {
	vmInstance := ctx.VM

	// Get Object.prototype for inheritance
	objectProto := vmInstance.ObjectPrototype

	// Create RegExp.prototype inheriting from Object.prototype
	regexpProto := vm.NewObject(objectProto).AsPlainObject()

	// Add RegExp prototype methods
	regexpProto.SetOwnNonEnumerable("test", vm.NewNativeFunction(1, false, "test", func(args []vm.Value) (vm.Value, error) {
		thisRegex := vmInstance.GetThis()
		if !thisRegex.IsRegExp() {
			return vm.Undefined, nil
		}
		regex := thisRegex.AsRegExpObject()
		// Check for deferred compile error
		if regex.HasCompileError() {
			return vm.Undefined, fmt.Errorf("SyntaxError: Invalid regular expression: %s", regex.GetCompileError())
		}
		if len(args) == 0 {
			return vm.BooleanValue(false), nil
		}
		str := args[0].ToString()
		matched := regex.MatchString(str)
		return vm.BooleanValue(matched), nil
	}))

	regexpProto.SetOwnNonEnumerable("exec", vm.NewNativeFunction(1, false, "exec", func(args []vm.Value) (vm.Value, error) {
		thisRegex := vmInstance.GetThis()
		if !thisRegex.IsRegExp() {
			return vm.Undefined, nil
		}
		regex := thisRegex.AsRegExpObject()
		// Check for deferred compile error
		if regex.HasCompileError() {
			return vm.Undefined, fmt.Errorf("SyntaxError: Invalid regular expression: %s", regex.GetCompileError())
		}
		if len(args) == 0 {
			return vm.Null, nil
		}
		str := args[0].ToString()

		var matches []string
		if regex.IsGlobal() {
			// Global regex: use lastIndex for stateful matching
			remainder := str[regex.GetLastIndex():]
			if loc := regex.FindStringSubmatchIndex(remainder); loc != nil {
				matches = regex.FindStringSubmatch(remainder)
				// Update lastIndex
				regex.SetLastIndex(regex.GetLastIndex() + loc[1])
			}
		} else {
			// Non-global: find first match
			matches = regex.FindStringSubmatch(str)
		}

		if matches == nil {
			if regex.IsGlobal() {
				regex.SetLastIndex(0) // Reset lastIndex on failure
			}
			return vm.Null, nil
		}

		// Create result array with matches
		result := vm.NewArray()
		arr := result.AsArray()
		for _, match := range matches {
			arr.Append(vm.NewString(match))
		}
		return result, nil
	}))

	regexpProto.SetOwnNonEnumerable("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) (vm.Value, error) {
		thisRegex := vmInstance.GetThis()
		if !thisRegex.IsRegExp() {
			return vm.Undefined, nil
		}
		regex := thisRegex.AsRegExpObject()
		result := "/" + regex.GetSource() + "/" + regex.GetFlags()
		return vm.NewString(result), nil
	}))

	// RegExp.prototype[@@search] ( string )
	// Returns the index of the first match of the regexp in the string, or -1 if not found
	searchFunc := vm.NewNativeFunction(1, false, "[Symbol.search]", func(args []vm.Value) (vm.Value, error) {
		thisRegex := vmInstance.GetThis()
		if !thisRegex.IsRegExp() {
			return vm.Undefined, vmInstance.NewTypeError("RegExp.prototype[@@search] requires a RegExp")
		}
		regex := thisRegex.AsRegExpObject()
		// Check for deferred compile error
		if regex.HasCompileError() {
			return vm.Undefined, fmt.Errorf("SyntaxError: Invalid regular expression: %s", regex.GetCompileError())
		}

		// Get string argument with proper ToString conversion (ECMAScript step 3)
		var str string
		if len(args) > 0 {
			arg := args[0]
			// Handle boxed String objects and other objects via ToPrimitive
			if arg.IsObject() {
				// Check for [[PrimitiveValue]] (String wrapper)
				if plainObj := arg.AsPlainObject(); plainObj != nil {
					if primitiveVal, exists := plainObj.GetOwn("[[PrimitiveValue]]"); exists && primitiveVal.Type() == vm.TypeString {
						str = primitiveVal.ToString()
					} else {
						// For other objects, use ToPrimitive
						vmInstance.EnterHelperCall()
						primVal := vmInstance.ToPrimitive(arg, "string")
						vmInstance.ExitHelperCall()
						if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
							return vm.Undefined, nil
						}
						str = primVal.ToString()
					}
				} else {
					str = arg.ToString()
				}
			} else {
				str = arg.ToString()
			}
		}

		// Save and reset lastIndex
		previousLastIndex := regex.GetLastIndex()
		if previousLastIndex != 0 {
			regex.SetLastIndex(0)
		}

		// Execute the regex
		loc := regex.FindStringIndex(str)

		// Restore lastIndex if it changed
		currentLastIndex := regex.GetLastIndex()
		if currentLastIndex != previousLastIndex {
			regex.SetLastIndex(previousLastIndex)
		}

		// Return -1 if no match, otherwise return the index
		if loc == nil {
			return vm.NumberValue(-1), nil
		}
		return vm.NumberValue(float64(loc[0])), nil
	})
	w, e, c := true, false, true // writable, not enumerable, configurable
	regexpProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolSearch), searchFunc, &w, &e, &c)

	// RegExp.prototype[@@matchAll] ( string )
	// Returns an iterator of all regex matches in the string
	matchAllFunc := vm.NewNativeFunction(1, false, "[Symbol.matchAll]", func(args []vm.Value) (vm.Value, error) {
		thisRegex := vmInstance.GetThis()
		if !thisRegex.IsRegExp() {
			return vm.Undefined, vmInstance.NewTypeError("RegExp.prototype[@@matchAll] requires a RegExp")
		}
		regex := thisRegex.AsRegExpObject()
		// Check for deferred compile error
		if regex.HasCompileError() {
			return vm.Undefined, fmt.Errorf("SyntaxError: Invalid regular expression: %s", regex.GetCompileError())
		}

		// Get string argument with proper ToString conversion
		var str string
		if len(args) > 0 {
			arg := args[0]
			// Handle boxed String objects and other objects via ToPrimitive
			if arg.IsObject() {
				if plainObj := arg.AsPlainObject(); plainObj != nil {
					if primitiveVal, exists := plainObj.GetOwn("[[PrimitiveValue]]"); exists && primitiveVal.Type() == vm.TypeString {
						str = primitiveVal.ToString()
					} else {
						vmInstance.EnterHelperCall()
						primVal := vmInstance.ToPrimitive(arg, "string")
						vmInstance.ExitHelperCall()
						if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
							return vm.Undefined, nil
						}
						str = primVal.ToString()
					}
				} else {
					str = arg.ToString()
				}
			} else {
				str = arg.ToString()
			}
		}

		// Find all matches with indices
		allMatches := regex.FindAllStringSubmatchIndex(str, -1)

		// Create and return a RegExp String Iterator (using the same iterator as String.prototype.matchAll)
		return createMatchAllIterator(vmInstance, str, allMatches), nil
	})
	regexpProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolMatchAll), matchAllFunc, &w, &e, &c)

	// RegExp.prototype[@@match] ( string )
	// Returns an array containing match results or null if no match
	matchFunc := vm.NewNativeFunction(1, false, "[Symbol.match]", func(args []vm.Value) (vm.Value, error) {
		thisRegex := vmInstance.GetThis()
		if !thisRegex.IsRegExp() {
			return vm.Undefined, vmInstance.NewTypeError("RegExp.prototype[@@match] requires a RegExp")
		}
		regex := thisRegex.AsRegExpObject()
		// Check for deferred compile error
		if regex.HasCompileError() {
			return vm.Undefined, fmt.Errorf("SyntaxError: Invalid regular expression: %s", regex.GetCompileError())
		}

		// Get string argument with proper ToString conversion
		var str string
		if len(args) > 0 {
			arg := args[0]
			if arg.IsObject() {
				if plainObj := arg.AsPlainObject(); plainObj != nil {
					if primitiveVal, exists := plainObj.GetOwn("[[PrimitiveValue]]"); exists && primitiveVal.Type() == vm.TypeString {
						str = primitiveVal.ToString()
					} else {
						vmInstance.EnterHelperCall()
						primVal := vmInstance.ToPrimitive(arg, "string")
						vmInstance.ExitHelperCall()
						if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
							return vm.Undefined, nil
						}
						str = primVal.ToString()
					}
				} else {
					str = arg.ToString()
				}
			} else {
				str = arg.ToString()
			}
		}

		if regex.IsGlobal() {
			// Global match: find all matches, return array of matched strings
			// Reset lastIndex to 0
			regex.SetLastIndex(0)

			matches := regex.FindAllString(str, -1)
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
			// Non-global: find first match, return array with match, groups, index, input
			loc := regex.FindStringSubmatchIndex(str)
			if loc == nil {
				return vm.Null, nil
			}

			matches := regex.FindStringSubmatch(str)

			// Create result array
			result := vm.NewArray()
			arr := result.AsArray()

			// Add matches (first is full match, rest are capture groups)
			for i, match := range matches {
				if i == 0 {
					arr.Append(vm.NewString(match))
				} else if loc[i*2] == -1 {
					// Unmatched capture group
					arr.Append(vm.Undefined)
				} else {
					arr.Append(vm.NewString(match))
				}
			}

			// Add index property (position of match in string)
			arr.SetOwn("index", vm.NumberValue(float64(loc[0])))
			arr.SetOwn("input", vm.NewString(str))

			return result, nil
		}
	})
	regexpProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolMatch), matchFunc, &w, &e, &c)

	// RegExp.prototype[@@replace] ( string, replaceValue )
	// Returns a new string with matches replaced
	replaceFunc := vm.NewNativeFunction(2, false, "[Symbol.replace]", func(args []vm.Value) (vm.Value, error) {
		thisRegex := vmInstance.GetThis()
		if !thisRegex.IsRegExp() {
			return vm.Undefined, vmInstance.NewTypeError("RegExp.prototype[@@replace] requires a RegExp")
		}
		regex := thisRegex.AsRegExpObject()
		// Check for deferred compile error
		if regex.HasCompileError() {
			return vm.Undefined, fmt.Errorf("SyntaxError: Invalid regular expression: %s", regex.GetCompileError())
		}

		// Get string argument with proper ToString conversion
		var str string
		if len(args) > 0 {
			arg := args[0]
			if arg.IsObject() {
				if plainObj := arg.AsPlainObject(); plainObj != nil {
					if primitiveVal, exists := plainObj.GetOwn("[[PrimitiveValue]]"); exists && primitiveVal.Type() == vm.TypeString {
						str = primitiveVal.ToString()
					} else {
						vmInstance.EnterHelperCall()
						primVal := vmInstance.ToPrimitive(arg, "string")
						vmInstance.ExitHelperCall()
						if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
							return vm.Undefined, nil
						}
						str = primVal.ToString()
					}
				} else {
					str = arg.ToString()
				}
			} else {
				str = arg.ToString()
			}
		}

		// Get replace value (can be string or function)
		var replaceValue vm.Value
		if len(args) > 1 {
			replaceValue = args[1]
		} else {
			replaceValue = vm.Undefined
		}

		isGlobal := regex.IsGlobal()

		// Check if replaceValue is callable
		isCallable := replaceValue.IsCallable()

		if isGlobal {
			// Reset lastIndex for global regexes
			regex.SetLastIndex(0)
		}

		// Find all matches (for global) or first match (for non-global)
		var result strings.Builder
		lastIndex := 0

		if isGlobal {
			allMatches := regex.FindAllStringSubmatchIndex(str, -1)
			for _, match := range allMatches {
				// Add the part before the match
				result.WriteString(str[lastIndex:match[0]])

				var replacement string
				if isCallable {
					// Call the replacer function with: match, p1, p2, ..., offset, string
					callArgs := make([]vm.Value, 0, len(match)/2+2)
					// Add full match
					callArgs = append(callArgs, vm.NewString(str[match[0]:match[1]]))
					// Add capture groups
					for i := 2; i < len(match); i += 2 {
						if match[i] >= 0 && match[i+1] >= 0 {
							callArgs = append(callArgs, vm.NewString(str[match[i]:match[i+1]]))
						} else {
							callArgs = append(callArgs, vm.Undefined)
						}
					}
					// Add offset and original string
					callArgs = append(callArgs, vm.NumberValue(float64(match[0])))
					callArgs = append(callArgs, vm.NewString(str))

					vmInstance.EnterHelperCall()
					res, err := vmInstance.Call(replaceValue, vm.Undefined, callArgs)
					vmInstance.ExitHelperCall()
					if err != nil {
						return vm.Undefined, err
					}
					replacement = res.ToString()
				} else {
					// Process replacement string with $ patterns
					replacement = processReplacementPattern(str, match, replaceValue.ToString())
				}
				result.WriteString(replacement)
				lastIndex = match[1]
			}
		} else {
			match := regex.FindStringSubmatchIndex(str)
			if match != nil {
				// Add the part before the match
				result.WriteString(str[lastIndex:match[0]])

				var replacement string
				if isCallable {
					// Call the replacer function with: match, p1, p2, ..., offset, string
					callArgs := make([]vm.Value, 0, len(match)/2+2)
					// Add full match
					callArgs = append(callArgs, vm.NewString(str[match[0]:match[1]]))
					// Add capture groups
					for i := 2; i < len(match); i += 2 {
						if match[i] >= 0 && match[i+1] >= 0 {
							callArgs = append(callArgs, vm.NewString(str[match[i]:match[i+1]]))
						} else {
							callArgs = append(callArgs, vm.Undefined)
						}
					}
					// Add offset and original string
					callArgs = append(callArgs, vm.NumberValue(float64(match[0])))
					callArgs = append(callArgs, vm.NewString(str))

					vmInstance.EnterHelperCall()
					res, err := vmInstance.Call(replaceValue, vm.Undefined, callArgs)
					vmInstance.ExitHelperCall()
					if err != nil {
						return vm.Undefined, err
					}
					replacement = res.ToString()
				} else {
					// Process replacement string with $ patterns
					replacement = processReplacementPattern(str, match, replaceValue.ToString())
				}
				result.WriteString(replacement)
				lastIndex = match[1]
			}
		}

		// Add the rest of the string after the last match
		result.WriteString(str[lastIndex:])
		return vm.NewString(result.String()), nil
	})
	regexpProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolReplace), replaceFunc, &w, &e, &c)

	// Create RegExp constructor function with properties
	regexpCtor := vm.NewConstructorWithProps(-1, true, "RegExp", func(args []vm.Value) (vm.Value, error) {
		// Constructor logic
		if len(args) == 0 {
			// new RegExp() - empty pattern
			result, err := vm.NewRegExp("(?:)", "")
			if err != nil {
				return vm.Undefined, fmt.Errorf("SyntaxError: %s", err.Error())
			}
			return result, nil
		}

		if len(args) == 1 {
			arg := args[0]
			if arg.IsRegExp() {
				// Copy constructor: new RegExp(regexObj)
				existing := arg.AsRegExpObject()
				result, err := vm.NewRegExp(existing.GetSource(), existing.GetFlags())
				if err != nil {
					return vm.Undefined, fmt.Errorf("SyntaxError: %s", err.Error())
				}
				return result, nil
			} else {
				// new RegExp(pattern) - convert to string and use empty flags
				pattern := arg.ToString()
				result, err := vm.NewRegExp(pattern, "")
				if err != nil {
					return vm.Undefined, fmt.Errorf("SyntaxError: %s", err.Error())
				}
				return result, nil
			}
		}

		// new RegExp(pattern, flags)
		pattern := args[0].ToString()
		flags := args[1].ToString()
		result, err := vm.NewRegExp(pattern, flags)
		if err != nil {
			return vm.Undefined, fmt.Errorf("SyntaxError: %s", err.Error())
		}
		return result, nil
	})

	// Set constructor property on RegExp.prototype to point to RegExp constructor
	regexpProto.SetOwnNonEnumerable("constructor", regexpCtor)
	if v, ok := regexpProto.GetOwn("constructor"); ok {
		w, e, c := true, false, true // writable, not enumerable, configurable
		regexpProto.DefineOwnProperty("constructor", v, &w, &e, &c)
	}

	// Register RegExp prototype in VM
	vmInstance.RegExpPrototype = vm.NewValueFromPlainObject(regexpProto)

	// Set up prototype relationship
	regexpCtor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(regexpProto))

	return ctx.DefineGlobal("RegExp", regexpCtor)
}

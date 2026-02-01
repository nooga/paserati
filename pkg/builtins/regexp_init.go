package builtins

import (
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

// processReplacementPatternEx processes $ patterns with captures as values
// This is used when we have capture values from regexpExec instead of indices
func processReplacementPatternEx(str string, matched string, position int, captures []vm.Value, namedCaptures vm.Value, replacement string) string {
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
				result.WriteString(matched)
				i += 2
			case '`':
				// $` -> portion before match
				if position <= len(str) {
					result.WriteString(str[:position])
				}
				i += 2
			case '\'':
				// $' -> portion after match
				endPos := position + len(matched)
				if endPos <= len(str) {
					result.WriteString(str[endPos:])
				}
				i += 2
			case '<':
				// $<name> -> named capture group
				if namedCaptures.Type() != vm.TypeUndefined && namedCaptures.IsObject() {
					// Find the closing >
					endIdx := i + 2
					for endIdx < len(replacement) && replacement[endIdx] != '>' {
						endIdx++
					}
					if endIdx < len(replacement) {
						name := replacement[i+2 : endIdx]
						if po := namedCaptures.AsPlainObject(); po != nil {
							if val, ok := po.GetOwn(name); ok && val.Type() != vm.TypeUndefined {
								result.WriteString(val.ToString())
							}
						}
						i = endIdx + 1
						continue
					}
				}
				// Invalid named capture, output literally
				result.WriteByte('$')
				i++
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
					if num, err := strconv.Atoi(numStr); err == nil && num > 0 {
						// Check if we have enough capture groups
						if num <= len(captures) {
							cap := captures[num-1]
							if cap.Type() != vm.TypeUndefined {
								result.WriteString(cap.ToString())
							}
							i = endIdx
							continue
						}
						// If two-digit didn't work, try one digit
						if len(numStr) == 2 {
							num, _ = strconv.Atoi(string(replacement[i+1]))
							if num > 0 && num <= len(captures) {
								cap := captures[num-1]
								if cap.Type() != vm.TypeUndefined {
									result.WriteString(cap.ToString())
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
			return vm.Undefined, vmInstance.NewSyntaxError("Invalid regular expression: " + regex.GetCompileError())
		}
		if len(args) == 0 {
			return vm.BooleanValue(false), nil
		}
		str := args[0].ToString()

		// Check for global and sticky flags
		flags := regex.GetFlags()
		isGlobal := strings.Contains(flags, "g")
		isSticky := strings.Contains(flags, "y")

		if isGlobal || isSticky {
			// Use lastIndex for stateful matching
			baseIndex := regex.GetLastIndex()
			if baseIndex < 0 {
				baseIndex = 0
			}
			if baseIndex > len(str) {
				// lastIndex beyond string length - no match possible
				regex.SetLastIndex(0)
				return vm.BooleanValue(false), nil
			}
			searchStr := str[baseIndex:]
			loc := regex.FindStringIndex(searchStr)

			// For sticky flag, match must occur at exactly position 0 of searchStr
			if isSticky && loc != nil && loc[0] != 0 {
				loc = nil
			}

			if loc != nil {
				// Update lastIndex
				regex.SetLastIndex(baseIndex + loc[1])
				return vm.BooleanValue(true), nil
			} else {
				// No match - reset lastIndex
				regex.SetLastIndex(0)
				return vm.BooleanValue(false), nil
			}
		}

		// Non-global, non-sticky: simple match
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
			return vm.Undefined, vmInstance.NewSyntaxError("Invalid regular expression: " + regex.GetCompileError())
		}
		if len(args) == 0 {
			return vm.Null, nil
		}
		str := args[0].ToString()

		// Check for global and sticky flags
		flags := regex.GetFlags()
		isGlobal := strings.Contains(flags, "g")
		isSticky := strings.Contains(flags, "y")

		var loc []int
		if isGlobal || isSticky {
			// Global or sticky regex: use lastIndex for stateful matching
			baseIndex := regex.GetLastIndex()
			if baseIndex < 0 {
				baseIndex = 0
			}
			if baseIndex > len(str) {
				// lastIndex beyond string length - no match possible
				regex.SetLastIndex(0)
				return vm.Null, nil
			}

			// Search full string to preserve lookbehind context
			// Find all matches and return the first one at or after baseIndex
			allMatches := regex.FindAllStringSubmatchIndex(str, -1)
			for _, match := range allMatches {
				if match[0] >= baseIndex {
					// For sticky, must match exactly at baseIndex
					if isSticky && match[0] != baseIndex {
						break // No valid sticky match
					}
					loc = match
					break
				}
			}

			if loc != nil {
				// Update lastIndex
				regex.SetLastIndex(loc[1])
			} else {
				// No match - reset lastIndex
				regex.SetLastIndex(0)
			}
		} else {
			// Non-global, non-sticky: find first match from beginning
			loc = regex.FindStringSubmatchIndex(str)
		}

		if loc == nil {
			return vm.Null, nil
		}

		// Create result array with matches
		// Use FindStringSubmatchIndex to detect non-participating groups
		// (they have indices of -1, -1)
		result := vm.NewArray()
		arr := result.AsArray()
		// loc contains pairs of indices [start0, end0, start1, end1, ...]
		for i := 0; i < len(loc); i += 2 {
			start, end := loc[i], loc[i+1]
			if start == -1 {
				// Non-participating group: use undefined (JavaScript semantics)
				arr.Append(vm.Undefined)
			} else {
				arr.Append(vm.NewString(str[start:end]))
			}
		}
		// Set required properties: index, input, groups
		// loc indices are already absolute positions in str
		arr.SetOwn("index", vm.NumberValue(float64(loc[0])))
		arr.SetOwn("input", vm.NewString(str))
		arr.SetOwn("groups", vm.Undefined) // TODO: Named capture groups
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

	w, e, c := true, false, true // writable, not enumerable, configurable

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
			return vm.Undefined, vmInstance.NewSyntaxError("Invalid regular expression: " + regex.GetCompileError())
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

	// Helper to convert a value to string using JavaScript semantics (calls toString for objects)
	toStringJS := func(val vm.Value) string {
		if val.Type() == vm.TypeUndefined {
			return "undefined"
		}
		if val.Type() == vm.TypeNull {
			return "null"
		}
		if val.Type() == vm.TypeString {
			return val.ToString()
		}
		if val.IsObject() {
			// Check for [[PrimitiveValue]] (String wrapper)
			if plainObj := val.AsPlainObject(); plainObj != nil {
				if primitiveVal, exists := plainObj.GetOwn("[[PrimitiveValue]]"); exists && primitiveVal.Type() == vm.TypeString {
					return primitiveVal.ToString()
				}
			}
			// For other objects, use ToPrimitive
			vmInstance.EnterHelperCall()
			primVal := vmInstance.ToPrimitive(val, "string")
			vmInstance.ExitHelperCall()
			if vmInstance.IsUnwinding() || vmInstance.IsHandlerFound() {
				return ""
			}
			return primVal.ToString()
		}
		return val.ToString()
	}

	// Helper: advanceStringIndex - advance by one code point for unicode mode
	advanceStringIndex := func(str string, index int, unicode bool) int {
		if !unicode {
			return index + 1
		}
		// For unicode mode, advance by one code point (may be 2 UTF-16 code units for surrogate pairs)
		if index >= len(str) {
			return index + 1
		}
		// Get the rune at index
		runes := []rune(str)
		runeIndex := 0
		byteIndex := 0
		for byteIndex < index && runeIndex < len(runes) {
			byteIndex += len(string(runes[runeIndex]))
			runeIndex++
		}
		if runeIndex < len(runes) {
			return byteIndex + len(string(runes[runeIndex]))
		}
		return index + 1
	}

	// Helper: RegExpExec (R, S) - ES2023 22.2.7.1
	// Calls exec property if callable, otherwise uses built-in exec
	regexpExec := func(rx vm.Value, str string) (vm.Value, error) {
		// Step 3: Get exec property
		execProp, err := vmInstance.GetProperty(rx, "exec")
		if err != nil {
			return vm.Undefined, err
		}

		// Step 4: If exec is callable, call it
		if execProp.IsCallable() {
			vmInstance.EnterHelperCall()
			result, callErr := vmInstance.Call(execProp, rx, []vm.Value{vm.NewString(str)})
			vmInstance.ExitHelperCall()
			if vmInstance.IsUnwinding() {
				return vm.Undefined, nil
			}
			if callErr != nil {
				return vm.Undefined, callErr
			}
			// Step 4b: Result must be Object or null
			if result.Type() != vm.TypeNull && !result.IsObject() && result.Type() != vm.TypeArray {
				return vm.Undefined, vmInstance.NewTypeError("RegExp exec must return object or null")
			}
			return result, nil
		}

		// Step 5: If R doesn't have [[RegExpMatcher]], throw TypeError
		if !rx.IsRegExp() {
			return vm.Undefined, vmInstance.NewTypeError("RegExp.prototype method called on non-RegExp without exec")
		}

		// Step 6: Use built-in exec (RegExpBuiltinExec)
		regex := rx.AsRegExpObject()
		if regex.HasCompileError() {
			return vm.Undefined, vmInstance.NewSyntaxError("Invalid regular expression: " + regex.GetCompileError())
		}

		// Get lastIndex and flags for sticky handling
		lastIndex := regex.GetLastIndex()
		flags := regex.GetFlags()
		isSticky := strings.Contains(flags, "y")
		isGlobal := regex.IsGlobal()

		// For non-global, non-sticky, start from beginning
		searchStart := 0
		if isSticky || isGlobal {
			searchStart = lastIndex
			if searchStart < 0 {
				searchStart = 0
			}
		}

		// If lastIndex > string length, fail
		if searchStart > len(str) {
			if isSticky || isGlobal {
				regex.SetLastIndex(0)
			}
			return vm.Null, nil
		}

		// Execute match - for sticky/global, search in substring starting at searchStart
		var loc []int
		if searchStart > 0 {
			subLoc := regex.FindStringSubmatchIndex(str[searchStart:])
			if subLoc != nil {
				// Adjust indices to original string
				loc = make([]int, len(subLoc))
				for i := range subLoc {
					if subLoc[i] >= 0 {
						loc[i] = subLoc[i] + searchStart
					} else {
						loc[i] = subLoc[i]
					}
				}
			}
		} else {
			loc = regex.FindStringSubmatchIndex(str)
		}

		if loc == nil {
			if isSticky || isGlobal {
				regex.SetLastIndex(0)
			}
			return vm.Null, nil
		}

		// For sticky, match must start exactly at lastIndex
		if isSticky && loc[0] != searchStart {
			regex.SetLastIndex(0)
			return vm.Null, nil
		}

		// Update lastIndex for global/sticky
		if isSticky || isGlobal {
			regex.SetLastIndex(loc[1])
		}

		// Build result array
		result := vm.NewArray()
		arr := result.AsArray()
		for i := 0; i < len(loc); i += 2 {
			start, end := loc[i], loc[i+1]
			if start == -1 {
				arr.Append(vm.Undefined)
			} else {
				arr.Append(vm.NewString(str[start:end]))
			}
		}
		arr.SetOwn("index", vm.NumberValue(float64(loc[0])))
		arr.SetOwn("input", vm.NewString(str))
		arr.SetOwn("groups", vm.Undefined)

		return result, nil
	}

	// RegExp.prototype[@@match] ( string ) - ES2023 22.2.6.8
	// Returns an array containing match results or null if no match
	matchFunc := vm.NewNativeFunction(1, false, "[Symbol.match]", func(args []vm.Value) (vm.Value, error) {
		rx := vmInstance.GetThis()

		// Step 2: If Type(rx) is not Object, throw TypeError
		if !rx.IsObject() && !rx.IsRegExp() && rx.Type() != vm.TypeArray {
			return vm.Undefined, vmInstance.NewTypeError("RegExp.prototype[@@match] called on non-object")
		}

		// Step 3: ToString(string) - use JavaScript semantics for objects
		var str string
		if len(args) > 0 {
			str = toStringJS(args[0])
		}

		// Step 4: Get flags
		flagsVal, err := vmInstance.GetProperty(rx, "flags")
		if err != nil {
			return vm.Undefined, err
		}
		flags := flagsVal.ToString()

		// Step 5: If not global, return RegExpExec(rx, S)
		isGlobal := strings.Contains(flags, "g")
		if !isGlobal {
			return regexpExec(rx, str)
		}

		// Step 6: Global match
		// Step 6a: Check for unicode
		fullUnicode := strings.Contains(flags, "u")

		// Step 6b: Set lastIndex to 0
		if err := vmInstance.SetProperty(rx, "lastIndex", vm.NumberValue(0)); err != nil {
			return vm.Undefined, err
		}

		// Step 6c: Create result array
		resultArr := vm.NewArray()
		arr := resultArr.AsArray()
		n := 0

		// Step 6e: Loop
		for {
			// Step 6e.i: Call RegExpExec
			result, execErr := regexpExec(rx, str)
			if execErr != nil {
				return vm.Undefined, execErr
			}
			if vmInstance.IsUnwinding() {
				return vm.Undefined, nil
			}

			// Step 6e.ii: If result is null
			if result.Type() == vm.TypeNull {
				if n == 0 {
					return vm.Null, nil
				}
				return resultArr, nil
			}

			// Step 6e.iii: Get match string (result[0])
			matchStrVal, err := vmInstance.GetProperty(result, "0")
			if err != nil {
				return vm.Undefined, err
			}
			matchStr := matchStrVal.ToString()

			// Step 6e.iv: Add to result array
			arr.Append(vm.NewString(matchStr))

			// Step 6e.v: If matchStr is empty, advance lastIndex
			if matchStr == "" {
				lastIndexVal, _ := vmInstance.GetProperty(rx, "lastIndex")
				thisIndex := int(lastIndexVal.ToFloat())
				nextIndex := advanceStringIndex(str, thisIndex, fullUnicode)
				if err := vmInstance.SetProperty(rx, "lastIndex", vm.NumberValue(float64(nextIndex))); err != nil {
					return vm.Undefined, err
				}
			}

			n++

			// Safety limit to prevent infinite loops
			if n > 1000000 {
				return vm.Undefined, vmInstance.NewRangeError("Maximum match iterations exceeded")
			}
		}
	})
	regexpProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolMatch), matchFunc, &w, &e, &c)

	// RegExp.prototype[@@search] ( string ) - ES2023 22.2.6.9
	// Returns the index of the first match of the regexp in the string, or -1 if not found
	searchFunc := vm.NewNativeFunction(1, false, "[Symbol.search]", func(args []vm.Value) (vm.Value, error) {
		rx := vmInstance.GetThis()

		// Step 2: If Type(rx) is not Object, throw TypeError
		if !rx.IsObject() && !rx.IsRegExp() && rx.Type() != vm.TypeArray {
			return vm.Undefined, vmInstance.NewTypeError("RegExp.prototype[@@search] called on non-object")
		}

		// Step 3: ToString(string) - use JavaScript semantics for objects
		var str string
		if len(args) > 0 {
			str = toStringJS(args[0])
		}

		// Step 4: Get previous lastIndex
		previousLastIndex, _ := vmInstance.GetProperty(rx, "lastIndex")

		// Step 5: If previous lastIndex is not 0, set to 0
		if previousLastIndex.ToFloat() != 0 {
			if err := vmInstance.SetProperty(rx, "lastIndex", vm.NumberValue(0)); err != nil {
				return vm.Undefined, err
			}
		}

		// Step 6: Call RegExpExec
		result, execErr := regexpExec(rx, str)
		if execErr != nil {
			return vm.Undefined, execErr
		}
		if vmInstance.IsUnwinding() {
			return vm.Undefined, nil
		}

		// Step 7: Get current lastIndex
		currentLastIndex, _ := vmInstance.GetProperty(rx, "lastIndex")

		// Step 8: If currentLastIndex !== previousLastIndex, restore it
		if currentLastIndex.ToFloat() != previousLastIndex.ToFloat() {
			if err := vmInstance.SetProperty(rx, "lastIndex", previousLastIndex); err != nil {
				return vm.Undefined, err
			}
		}

		// Step 9: If result is null, return -1
		if result.Type() == vm.TypeNull {
			return vm.NumberValue(-1), nil
		}

		// Step 10: Return result.index
		indexVal, _ := vmInstance.GetProperty(result, "index")
		return vm.NumberValue(indexVal.ToFloat()), nil
	})
	regexpProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolSearch), searchFunc, &w, &e, &c)

	// RegExp.prototype[@@replace] ( string, replaceValue ) - ES2023 22.2.6.10
	// Returns a new string with matches replaced
	replaceFunc := vm.NewNativeFunction(2, false, "[Symbol.replace]", func(args []vm.Value) (vm.Value, error) {
		rx := vmInstance.GetThis()

		// Step 2: If Type(rx) is not Object, throw TypeError
		if !rx.IsObject() && !rx.IsRegExp() && rx.Type() != vm.TypeArray {
			return vm.Undefined, vmInstance.NewTypeError("RegExp.prototype[@@replace] called on non-object")
		}

		// Step 3: ToString(string) - use JavaScript semantics for objects
		var str string
		if len(args) > 0 {
			str = toStringJS(args[0])
		}

		// Step 4: Get replace value (can be string or function)
		var replaceValue vm.Value
		if len(args) > 1 {
			replaceValue = args[1]
		} else {
			replaceValue = vm.Undefined
		}

		// Step 5: Check if replaceValue is callable
		isCallable := replaceValue.IsCallable()

		// Step 6: Get flags
		flagsVal, err := vmInstance.GetProperty(rx, "flags")
		if err != nil {
			return vm.Undefined, err
		}
		flags := flagsVal.ToString()

		// Step 7-8: Check for global and unicode flags
		isGlobal := strings.Contains(flags, "g")
		fullUnicode := strings.Contains(flags, "u")

		// Step 9: If global, reset lastIndex to 0
		if isGlobal {
			if err := vmInstance.SetProperty(rx, "lastIndex", vm.NumberValue(0)); err != nil {
				return vm.Undefined, err
			}
		}

		// Step 10-11: Collect all results
		var results []vm.Value
		for {
			result, execErr := regexpExec(rx, str)
			if execErr != nil {
				return vm.Undefined, execErr
			}
			if vmInstance.IsUnwinding() {
				return vm.Undefined, nil
			}

			if result.Type() == vm.TypeNull {
				break
			}

			results = append(results, result)

			if !isGlobal {
				break
			}

			// Step 11c: Get match string for global case
			matchStrVal, _ := vmInstance.GetProperty(result, "0")
			matchStr := matchStrVal.ToString()

			// Step 11c.iii: If match is empty string, advance lastIndex
			if matchStr == "" {
				lastIndexVal, _ := vmInstance.GetProperty(rx, "lastIndex")
				thisIndex := int(lastIndexVal.ToFloat())
				nextIndex := advanceStringIndex(str, thisIndex, fullUnicode)
				if err := vmInstance.SetProperty(rx, "lastIndex", vm.NumberValue(float64(nextIndex))); err != nil {
					return vm.Undefined, err
				}
			}
		}

		// Step 12: Build result string
		accumulatedResult := ""
		nextSourcePosition := 0

		// Step 13: Process each result
		for _, result := range results {
			// Step 13a: Get number of captures
			lenVal, _ := vmInstance.GetProperty(result, "length")
			nCaptures := int(lenVal.ToFloat()) - 1
			if nCaptures < 0 {
				nCaptures = 0
			}

			// Step 13b: Get matched substring
			matchedVal, _ := vmInstance.GetProperty(result, "0")
			matched := matchedVal.ToString()

			// Step 13c: Get position
			positionVal, _ := vmInstance.GetProperty(result, "index")
			position := int(positionVal.ToFloat())
			if position < 0 {
				position = 0
			}
			if position > len(str) {
				position = len(str)
			}

			// Step 13d-e: Get capture groups
			captures := make([]vm.Value, nCaptures)
			for n := 1; n <= nCaptures; n++ {
				capVal, _ := vmInstance.GetProperty(result, strconv.Itoa(n))
				if capVal.Type() == vm.TypeUndefined {
					captures[n-1] = vm.Undefined
				} else {
					captures[n-1] = vm.NewString(capVal.ToString())
				}
			}

			// Step 13f-g: Get named captures (if present)
			namedCaptures, _ := vmInstance.GetProperty(result, "groups")

			// Step 13h-k: Compute replacement
			var replacement string
			if isCallable {
				// Build replacer arguments: matched, p1, p2, ..., position, str, [namedCaptures]
				replacerArgs := make([]vm.Value, 0, len(captures)+3)
				replacerArgs = append(replacerArgs, vm.NewString(matched))
				replacerArgs = append(replacerArgs, captures...)
				replacerArgs = append(replacerArgs, vm.NumberValue(float64(position)))
				replacerArgs = append(replacerArgs, vm.NewString(str))
				if namedCaptures.Type() != vm.TypeUndefined {
					replacerArgs = append(replacerArgs, namedCaptures)
				}

				vmInstance.EnterHelperCall()
				replResult, callErr := vmInstance.Call(replaceValue, vm.Undefined, replacerArgs)
				vmInstance.ExitHelperCall()
				if callErr != nil {
					return vm.Undefined, callErr
				}
				replacement = replResult.ToString()
			} else {
				// Use GetSubstitution for string replacement
				// Build match indices for $ substitution processing
				matchIndices := []int{position, position + len(matched)}
				for _, cap := range captures {
					if cap.Type() == vm.TypeUndefined {
						matchIndices = append(matchIndices, -1, -1)
					} else {
						// For captures we don't have position info, use -1
						matchIndices = append(matchIndices, -1, -1)
					}
				}
				replacement = processReplacementPatternEx(str, matched, position, captures, namedCaptures, replaceValue.ToString())
			}

			// Step 13l-m: Update accumulated result
			if position >= nextSourcePosition {
				accumulatedResult += str[nextSourcePosition:position] + replacement
				nextSourcePosition = position + len(matched)
			}
		}

		// Step 14: Append remaining portion of str
		if nextSourcePosition < len(str) {
			accumulatedResult += str[nextSourcePosition:]
		}

		return vm.NewString(accumulatedResult), nil
	})
	regexpProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolReplace), replaceFunc, &w, &e, &c)

	// RegExp.prototype[@@split] ( string, limit ) - ES2023 22.2.6.13
	// Splits a string using a regular expression
	splitFunc := vm.NewNativeFunction(2, false, "[Symbol.split]", func(args []vm.Value) (vm.Value, error) {
		rx := vmInstance.GetThis()

		// Step 2: If Type(rx) is not Object, throw TypeError
		if !rx.IsObject() && !rx.IsRegExp() && rx.Type() != vm.TypeArray {
			return vm.Undefined, vmInstance.NewTypeError("RegExp.prototype[@@split] called on non-object")
		}

		// Step 3: ToString(string) - use JavaScript semantics
		var str string
		if len(args) > 0 {
			str = toStringJS(args[0])
		}

		// Step 4-6: Get constructor and create splitter
		// For simplicity, we'll use the rx directly as the splitter

		// Step 7: Get flags
		flagsVal, err := vmInstance.GetProperty(rx, "flags")
		if err != nil {
			return vm.Undefined, err
		}
		flags := flagsVal.ToString()

		// Step 8: Check for unicode flag
		unicodeMatching := strings.Contains(flags, "u")

		// Step 9-10: Add sticky flag if not present (for our matching logic)
		// We'll handle sticky logic manually

		// Step 11: Create result array
		resultArr := vm.NewArray()
		arr := resultArr.AsArray()

		// Step 12: Limit handling
		var lim uint32 = 0xFFFFFFFF // unlimited
		if len(args) >= 2 && args[1].Type() != vm.TypeUndefined {
			lim = uint32(args[1].ToFloat())
		}

		// Step 13: If limit is 0, return empty array
		if lim == 0 {
			return resultArr, nil
		}

		// Step 14: If string is empty
		if len(str) == 0 {
			// Step 14a: Execute match on empty string
			if err := vmInstance.SetProperty(rx, "lastIndex", vm.NumberValue(0)); err != nil {
				return vm.Undefined, err
			}
			result, execErr := regexpExec(rx, str)
			if execErr != nil {
				return vm.Undefined, execErr
			}
			if vmInstance.IsUnwinding() {
				return vm.Undefined, nil
			}
			// Step 14b: If result is not null, return empty array
			if result.Type() != vm.TypeNull {
				return resultArr, nil
			}
			// Step 14c: Add empty string to result
			arr.Append(vm.NewString(""))
			return resultArr, nil
		}

		// Step 15-23: Main split loop
		p := 0 // position of last match end
		q := 0 // current search position

		for q < len(str) {
			// Step 19: Set lastIndex to 0 for the substring search
			if err := vmInstance.SetProperty(rx, "lastIndex", vm.NumberValue(0)); err != nil {
				return vm.Undefined, err
			}

			// Step 20: Execute match on substring starting at q
			// This handles non-global regexes that ignore lastIndex
			substr := str[q:]
			result, execErr := regexpExec(rx, substr)
			if execErr != nil {
				return vm.Undefined, execErr
			}
			if vmInstance.IsUnwinding() {
				return vm.Undefined, nil
			}

			// Step 21: If no match, we're done - break out of loop
			if result.Type() == vm.TypeNull {
				break
			}

			// Get the actual match position (relative to substring)
			indexVal, _ := vmInstance.GetProperty(result, "index")
			relativeMatchStart := int(indexVal.ToFloat())

			// Per ES spec, split uses sticky matching - match must start at position q
			// If match doesn't start at beginning of substring, advance q and continue
			if relativeMatchStart != 0 {
				q = advanceStringIndex(str, q, unicodeMatching)
				continue
			}

			matchStart := q + relativeMatchStart

			// Step 22: Get the end position of the match
			matchedStrVal, _ := vmInstance.GetProperty(result, "0")
			matchedStr := matchedStrVal.ToString()
			e := matchStart + len(matchedStr)
			if e > len(str) {
				e = len(str)
			}

			// Step 23: If e == p, advance q and continue (prevent infinite loop on empty matches)
			if e == p {
				q = advanceStringIndex(str, q, unicodeMatching)
				continue
			}

			// Step 24: Add substring before match to result
			arr.Append(vm.NewString(str[p:matchStart]))

			// Step 25: Check limit
			if uint32(arr.Length()) == lim {
				return resultArr, nil
			}

			// Step 26: Update p to e
			p = e

			// Step 27-29: Add captured groups to result
			lenVal, _ := vmInstance.GetProperty(result, "length")
			numberOfCaptures := int(lenVal.ToFloat()) - 1
			if numberOfCaptures < 0 {
				numberOfCaptures = 0
			}

			for i := 1; i <= numberOfCaptures; i++ {
				capVal, _ := vmInstance.GetProperty(result, strconv.Itoa(i))
				arr.Append(capVal)

				// Check limit after each capture
				if uint32(arr.Length()) == lim {
					return resultArr, nil
				}
			}

			// Step 30: Set q to p
			q = p
		}

		// Step 31: Add remaining substring
		arr.Append(vm.NewString(str[p:]))

		return resultArr, nil
	})
	regexpProto.DefineOwnPropertyByKey(vm.NewSymbolKey(SymbolSplit), splitFunc, &w, &e, &c)

	// Create RegExp constructor function with properties
	regexpCtor := vm.NewConstructorWithProps(-1, true, "RegExp", func(args []vm.Value) (vm.Value, error) {
		// Constructor logic
		if len(args) == 0 {
			// new RegExp() - empty pattern
			result, err := vm.NewRegExp("(?:)", "")
			if err != nil {
				return vm.Undefined, vmInstance.NewSyntaxError(err.Error())
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
					return vm.Undefined, vmInstance.NewSyntaxError(err.Error())
				}
				return result, nil
			} else {
				// new RegExp(pattern) - convert to string and use empty flags
				pattern := arg.ToString()
				result, err := vm.NewRegExp(pattern, "")
				if err != nil {
					return vm.Undefined, vmInstance.NewSyntaxError(err.Error())
				}
				return result, nil
			}
		}

		// new RegExp(pattern, flags)
		pattern := args[0].ToString()
		flags := args[1].ToString()
		result, err := vm.NewRegExp(pattern, flags)
		if err != nil {
			return vm.Undefined, vmInstance.NewSyntaxError(err.Error())
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

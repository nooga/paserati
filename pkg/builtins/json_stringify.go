package builtins

import (
	"errors"
	"math"
	"strconv"
	"unsafe"

	"github.com/nooga/paserati/pkg/vm"
)

// stringifyValueToJSONWithVisited converts a VM Value to a JSON string with circular reference detection
// gap is the indentation string (e.g., "  " for 2 spaces), indent is the current indentation level
// key is the property key (for toJSON method calls)
// holder is the parent object containing this value
// replacerFunc is the replacer function (if any)
// propertyList is the property whitelist (if replacer is an array)
//
// The result is "" when the value serializes to undefined.
func stringifyValueToJSONWithVisited(vmInstance *vm.VM, value vm.Value, visited map[uintptr]bool, gap string, indent string, key string, holder vm.Value, replacerFunc vm.Value, propertyList []string) (string, error) {
	var out []byte
	ok, err := jsonAppendValue(vmInstance, value, visited, gap, indent, key, holder, replacerFunc, propertyList, &out)
	if err != nil || !ok {
		return "", err
	}
	return string(out), nil
}

// jsonAppendValue is SerializeJSONProperty writing into *out. It reports false,
// leaving *out as it found it, when the value serializes to undefined
// (undefined, symbols, functions): the caller then omits an object member or
// writes null for an array element. Appending into one buffer keeps the whole
// serialization linear; building each level's string and concatenating it into
// its parent copied every byte once per nesting level.
func jsonAppendValue(vmInstance *vm.VM, value vm.Value, visited map[uintptr]bool, gap string, indent string, key string, holder vm.Value, replacerFunc vm.Value, propertyList []string, out *[]byte) (bool, error) {
	// Step 0: Handle rawJSON objects (ES2024) - return rawJSON property directly
	if value.Type() == vm.TypeObject && len(rawJSONObjects) > 0 {
		obj := value.AsPlainObject()
		if obj != nil && rawJSONObjects[obj] {
			if rawJSON, ok := obj.GetOwn("rawJSON"); ok {
				*out = append(*out, rawJSON.ToString()...)
				return true, nil
			}
		}
	}

	// Step 1: Handle toJSON method if present (objects, arrays, proxies, and BigInt)
	// Per ECMAScript spec: If Type(value) is Object or BigInt, check for toJSON
	if value.Type() == vm.TypeObject || value.Type() == vm.TypeDictObject || value.Type() == vm.TypeArray || value.Type() == vm.TypeProxy || value.Type() == vm.TypeBigInt || value.Type() == vm.TypeTypedArray {
		var toJSON vm.Value
		var err error

		// Use GetProperty to properly invoke getters (which may throw)
		if vmInstance != nil {
			toJSON, err = vmInstance.GetProperty(value, "toJSON")
			if err != nil {
				return false, err
			}
		} else {
			// Fallback if no VM instance
			var ok bool
			if value.Type() == vm.TypeObject {
				toJSON, ok = value.AsPlainObject().GetOwn("toJSON")
			} else if value.Type() == vm.TypeDictObject {
				toJSON, ok = value.AsDictObject().GetOwn("toJSON")
			} else if value.Type() == vm.TypeArray {
				toJSON, ok = value.AsArray().GetOwn("toJSON")
			}
			if !ok {
				toJSON = vm.Undefined
			}
		}

		if toJSON != vm.Undefined && toJSON.IsCallable() {
			// Call toJSON method with key as argument
			if vmInstance != nil {
				result, err := vmInstance.Call(toJSON, value, []vm.Value{vm.NewString(key)})
				if err != nil {
					return false, err
				}
				value = result
			}
		}
	}

	// Step 2: Apply replacer function if present
	if replacerFunc != vm.Undefined && replacerFunc.IsCallable() && vmInstance != nil {
		result, err := vmInstance.CallArgs2(replacerFunc, holder, vm.NewString(key), value)
		if err != nil {
			return false, err
		}
		value = result

		// Check if replacer returned a rawJSON object (ES2024)
		if value.Type() == vm.TypeObject {
			obj := value.AsPlainObject()
			if obj != nil && rawJSONObjects[obj] {
				if rawJSON, ok := obj.GetOwn("rawJSON"); ok {
					*out = append(*out, rawJSON.ToString()...)
					return true, nil
				}
			}
		}
	}

	// Step 3: Handle boxed primitives (Boolean, Number, String, BigInt objects)
	// Per spec: ToNumber for Number objects, ToString for String objects, value for Boolean objects
	// BigInt objects throw TypeError
	if value.Type() == vm.TypeObject && vmInstance != nil {
		obj := value.AsPlainObject()
		if pv, ok := obj.GetInternal("[[PrimitiveValue]]"); ok {
			// This is a boxed primitive - convert using ToPrimitive
			switch pv.Type() {
			case vm.TypeFloatNumber, vm.TypeIntegerNumber:
				// Number object - call ToNumber via ToPrimitive with number hint
				value = vmInstance.ToPrimitive(value, "number")
			case vm.TypeString:
				// String object - call ToString via ToPrimitive with string hint
				value = vmInstance.ToPrimitive(value, "string")
			case vm.TypeBoolean:
				// Boolean object - just use the primitive value
				value = pv
			case vm.TypeBigInt:
				// BigInt objects cannot be serialized - throw TypeError
				return false, vmInstance.NewTypeError("Do not know how to serialize a BigInt")
			default:
				value = pv
			}
		}
	}

	switch value.Type() {
	case vm.TypeNull:
		*out = append(*out, "null"...)
		return true, nil
	case vm.TypeUndefined:
		return false, nil // JSON.stringify(undefined) returns undefined
	case vm.TypeSymbol:
		return false, nil // Symbols are not serializable
	case vm.TypeFunction, vm.TypeClosure, vm.TypeNativeFunction, vm.TypeNativeFunctionWithProps, vm.TypeBoundFunction, vm.TypeAsyncNativeFunction:
		return false, nil // Functions are not serializable
	case vm.TypeBigInt:
		// BigInt cannot be serialized in JSON - must throw TypeError
		if vmInstance != nil {
			return false, vmInstance.NewTypeError("Do not know how to serialize a BigInt")
		}
		return false, errors.New("TypeError: Do not know how to serialize a BigInt")
	case vm.TypeBoolean:
		if value.IsTruthy() {
			*out = append(*out, "true"...)
		} else {
			*out = append(*out, "false"...)
		}
		return true, nil
	case vm.TypeFloatNumber, vm.TypeIntegerNumber:
		num := value.ToFloat()
		// NaN and the infinities serialize as null
		if math.IsNaN(num) || math.IsInf(num, 0) {
			*out = append(*out, "null"...)
			return true, nil
		}
		// Negative zero serializes as "0", not "-0"
		if num == 0 {
			*out = append(*out, '0')
			return true, nil
		}
		if num == math.Trunc(num) && math.Abs(num) < 1e15 {
			*out = strconv.AppendInt(*out, int64(num), 10)
			return true, nil
		}
		*out = strconv.AppendFloat(*out, num, 'f', -1, 64)
		return true, nil
	case vm.TypeString:
		// Custom escaping to preserve lone surrogates
		*out = vm.AppendQuoteJSONString(*out, value.ToString())
		return true, nil
	case vm.TypeArray:
		arr := value.AsArray()
		if arr.Length() == 0 {
			*out = append(*out, "[]"...)
			return true, nil
		}

		// Check for circular reference
		ptr := uintptr(unsafe.Pointer(arr))
		if visited[ptr] {
			if vmInstance != nil {
				return false, vmInstance.NewTypeError("Converting circular structure to JSON")
			}
			return false, errors.New("TypeError: Converting circular structure to JSON")
		}
		visited[ptr] = true
		defer delete(visited, ptr) // Remove after processing to allow same object in different branches

		stepIndent := indent
		if gap != "" {
			stepIndent = indent + gap
		}
		*out = append(*out, '[')
		for i := 0; i < arr.Length(); i++ {
			if gap != "" {
				*out = append(*out, '\n')
				*out = append(*out, stepIndent...)
			} else if i > 0 {
				*out = append(*out, ',')
			}
			ok, err := jsonAppendValue(vmInstance, arr.Get(i), visited, gap, stepIndent, strconv.Itoa(i), value, replacerFunc, propertyList, out)
			if err != nil {
				return false, err
			}
			// In arrays, undefined/functions/symbols become "null"
			if !ok {
				*out = append(*out, "null"...)
			}
			if gap != "" && i < arr.Length()-1 {
				*out = append(*out, ',')
			}
		}
		if gap != "" {
			*out = append(*out, '\n')
			*out = append(*out, indent...)
		}
		*out = append(*out, ']')
		return true, nil
	case vm.TypeTypedArray:
		// TypedArrays are Integer-Indexed exotic objects: serialize like a
		// plain object keyed by their indices ({"0":1,"1":2,...}), per spec
		// SerializeJSONObject over [[OwnPropertyKeys]] (toJSON is handled
		// above). A replacer's propertyList (if given) picks the key set,
		// same as for a plain object - it is not limited to valid indices.
		ta := value.AsTypedArray()
		if ta == nil {
			*out = append(*out, "null"...)
			return true, nil
		}

		var keys []string
		if propertyList != nil {
			keys = propertyList
		} else {
			// Indices, then enumerable named properties (paserati#535).
			keys = typedArrayOwnNames(value, true)
		}

		getElem := func(key string) vm.Value {
			if vmInstance != nil {
				v, err := vmInstance.GetProperty(value, key)
				if err == nil {
					return v
				}
				return vm.Undefined
			}
			if idx, err := strconv.Atoi(key); err == nil && idx >= 0 && idx < ta.GetLength() {
				return ta.GetElement(idx)
			}
			return vm.Undefined
		}

		return jsonAppendMembers(out, keys, gap, indent, func(elemKey string, stepIndent string) (bool, error) {
			return jsonAppendValue(vmInstance, getElem(elemKey), visited, gap, stepIndent, elemKey, value, replacerFunc, propertyList, out)
		})
	case vm.TypeRegExp:
		// RegExp objects serialize as empty objects {}
		*out = append(*out, "{}"...)
		return true, nil
	case vm.TypeProxy:
		// Check if proxy is for an array (IsArray check)
		proxy := value.AsProxy()
		if proxy == nil {
			*out = append(*out, "null"...)
			return true, nil
		}
		if proxy.Revoked {
			if vmInstance != nil {
				return false, vmInstance.NewTypeError("Cannot perform 'ownKeys' on a proxy that has been revoked")
			}
			return false, errors.New("TypeError: Cannot perform 'ownKeys' on a proxy that has been revoked")
		}
		// Check if target is an array (recursively for proxy chains)
		if isProxyForArray(proxy) {
			// Serialize as an array using length and numeric indices
			s, err := stringifyProxyArray(vmInstance, value, visited, gap, indent, key, holder, replacerFunc, propertyList)
			if err != nil {
				return false, err
			}
			*out = append(*out, s...)
			return true, nil
		}
		// Fall through to object handling
		fallthrough
	case vm.TypeObject, vm.TypeDictObject, vm.TypeArguments, vm.TypeMap, vm.TypeSet, vm.TypePromise,
		vm.TypeWeakMap, vm.TypeWeakSet, vm.TypeWeakRef, vm.TypeFinalizationRegistry, vm.TypeArrayBuffer,
		vm.TypeSharedArrayBuffer, vm.TypeDataView, vm.TypeGenerator, vm.TypeAsyncGenerator:
		// arguments and the other ordinary-apart-from-internal-slots kinds
		// serialize their own enumerable string keys like any object; they
		// used to come out as "null" (paserati#535).
		// Get object pointer for circular reference check
		var ptr uintptr
		if value.Type() == vm.TypeObject {
			ptr = uintptr(unsafe.Pointer(value.AsPlainObject()))
		} else if value.Type() == vm.TypeDictObject {
			ptr = uintptr(unsafe.Pointer(value.AsDictObject()))
		} else if value.Type() == vm.TypeProxy {
			proxy := value.AsProxy()
			ptr = uintptr(unsafe.Pointer(proxy))
			// Already checked for revoked above
			if proxy != nil && proxy.Revoked {
				if vmInstance != nil {
					return false, vmInstance.NewTypeError("Cannot perform 'ownKeys' on a proxy that has been revoked")
				}
				return false, errors.New("TypeError: Cannot perform 'ownKeys' on a proxy that has been revoked")
			}
		} else {
			ptr = value.ObjectIdentity()
		}

		// Check for circular reference
		if visited[ptr] {
			if vmInstance != nil {
				return false, vmInstance.NewTypeError("Converting circular structure to JSON")
			}
			return false, errors.New("TypeError: Converting circular structure to JSON")
		}
		visited[ptr] = true
		defer delete(visited, ptr) // Remove after processing to allow same object in different branches

		// Get keys for the object
		var keys []string
		if propertyList != nil {
			// If replacer array provided, use its order and only include those keys
			keys = propertyList
		} else {
			// Get keys based on object type
			if value.Type() == vm.TypeObject {
				keys = sortJSONKeys(value.AsPlainObject().OwnKeys())
			} else if value.Type() == vm.TypeDictObject {
				keys = sortJSONKeys(value.AsDictObject().OwnKeys())
			} else if value.Type() == vm.TypeProxy && vmInstance != nil {
				// For proxies, recursively get keys handling proxy chains
				proxy := value.AsProxy()
				if proxy != nil && !proxy.Revoked {
					proxyKeys, err := getProxyOwnKeys(vmInstance, proxy)
					if err != nil {
						return false, err
					}
					keys = sortJSONKeys(proxyKeys)
				}
			} else if vmInstance != nil {
				// Other kinds: EnumerableOwnProperties via Object.keys.
				keysVal, err := objectKeysWithVM(vmInstance, []vm.Value{value})
				if err != nil {
					return false, err
				}
				if keysVal.Type() == vm.TypeArray {
					ka := keysVal.AsArray()
					for i := 0; i < ka.Length(); i++ {
						keys = append(keys, ka.Get(i).ToString())
					}
				}
			}
		}

		return jsonAppendMembers(out, keys, gap, indent, func(key string, stepIndent string) (bool, error) {
			// Use vm.GetProperty to properly invoke getters
			// Per spec, we must call the replacer for all keys in K, even if the property
			// has been deleted (returns undefined). The replacer may transform undefined to a value.
			var prop vm.Value
			if vmInstance != nil {
				var err error
				prop, err = vmInstance.GetProperty(value, key)
				if err != nil {
					return false, err
				}
			} else {
				// Fallback if no VM instance - get property directly from object
				if value.Type() == vm.TypeObject {
					prop, _ = value.AsPlainObject().GetOwn(key)
				} else if value.Type() == vm.TypeDictObject {
					prop, _ = value.AsDictObject().GetOwn(key)
				}
			}
			return jsonAppendValue(vmInstance, prop, visited, gap, stepIndent, key, value, replacerFunc, propertyList, out)
		})
	default:
		*out = append(*out, "null"...)
		return true, nil
	}
}

// jsonAppendMembers writes the {"key": value, ...} body shared by every
// object-like kind. member serializes one key's value straight into *out and
// reports whether it produced anything; a member that serializes to undefined
// is dropped, along with the separator and key written ahead of it.
func jsonAppendMembers(out *[]byte, keys []string, gap, indent string, member func(key, stepIndent string) (bool, error)) (bool, error) {
	stepIndent := indent
	if gap != "" {
		stepIndent = indent + gap
	}
	*out = append(*out, '{')
	first := true
	for _, key := range keys {
		mark := len(*out)
		if !first {
			*out = append(*out, ',')
		}
		if gap != "" {
			*out = append(*out, '\n')
			*out = append(*out, stepIndent...)
		}
		*out = vm.AppendQuoteJSONString(*out, key)
		*out = append(*out, ':')
		if gap != "" {
			*out = append(*out, ' ')
		}
		ok, err := member(key, stepIndent)
		if err != nil {
			return false, err
		}
		if !ok { // undefined (after any replacer ran): omit the member
			*out = (*out)[:mark]
			continue
		}
		first = false
	}
	if gap != "" && !first {
		*out = append(*out, '\n')
		*out = append(*out, indent...)
	}
	*out = append(*out, '}')
	return true, nil
}

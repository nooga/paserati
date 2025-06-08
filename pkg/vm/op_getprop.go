package vm

import "unicode/utf8"

func (vm *VM) opGetProp(ip int, objVal *Value, propName string, dest *Value) (bool, InterpretResult, Value) {

	// FIX: Use hash-based cache key to avoid collisions
	// Combine instruction pointer with property name hash
	propNameHash := 0
	for _, b := range []byte(propName) {
		propNameHash = propNameHash*31 + int(b)
	}
	cacheKey := (ip-5)*100000 + (propNameHash & 0xFFFF) // Use ip-5 since ip was advanced by 4
	cache, exists := vm.propCache[cacheKey]
	if !exists {
		cache = &PropInlineCache{
			state: CacheStateUninitialized,
		}
		vm.propCache[cacheKey] = cache
	}

	// Initialize prototypes if needed
	initPrototypes()

	// --- Special handling for .length ---
	// Check the *original* value type before checking if it's an Object type
	if propName == "length" {
		switch objVal.Type() {
		case TypeArray:
			arr := AsArray(*objVal)
			*dest = Number(float64(len(arr.elements)))
			return true, InterpretOK, *dest
		case TypeString:
			str := AsString(*objVal)
			// Use rune count for correct length of multi-byte strings
			*dest = Number(float64(utf8.RuneCountInString(str)))
			return true, InterpretOK, *dest
		}
		// If not Array or String, fall through to general object property lookup
	}

	// --- Handle prototype methods for primitives ---
	// Handle String prototype methods
	if objVal.IsString() {
		if method, exists := StringPrototype.GetOwn(propName); exists {
			*dest = createBoundMethod(*objVal, method)
			return true, InterpretOK, *dest
		}
	}

	// Handle Array prototype methods
	if objVal.IsArray() {
		if method, exists := ArrayPrototype.GetOwn(propName); exists {
			*dest = createBoundMethod(*objVal, method)
			return true, InterpretOK, *dest
		}
	}

	// Handle property access on NativeFunctionWithProps (like String.fromCharCode)
	if objVal.Type() == TypeNativeFunctionWithProps {
		nativeFnWithProps := objVal.AsNativeFunctionWithProps()
		if prop, exists := nativeFnWithProps.Properties.GetOwn(propName); exists {
			*dest = prop
			return true, InterpretOK, *dest
		}
	}

	// Handle property access on functions (including lazy .prototype)
	if objVal.Type() == TypeFunction {
		fn := AsFunction(*objVal)

		// Special handling for "prototype" property
		if propName == "prototype" {
			*dest = fn.getOrCreatePrototype()
			return true, InterpretOK, *dest
		}

		// Other function properties (if any)
		if fn.Properties != nil {
			if prop, exists := fn.Properties.GetOwn(propName); exists {
				*dest = prop
				return true, InterpretOK, *dest
			}
		}

		// Check function prototype methods
		if FunctionPrototype != nil {
			if method, exists := FunctionPrototype.GetOwn(propName); exists {
				*dest = createBoundMethod(*objVal, method)
				return true, InterpretOK, *dest
			}
		}

		*dest = Undefined
		return true, InterpretOK, *dest
	}

	// Handle property access on closures (delegate to underlying function)
	if objVal.Type() == TypeClosure {
		closure := AsClosure(*objVal)
		fn := closure.Fn

		// Special handling for "prototype" property
		if propName == "prototype" {
			*dest = fn.getOrCreatePrototype()
			return true, InterpretOK, *dest
		}

		// Other function properties (if any)
		if fn.Properties != nil {
			if prop, exists := fn.Properties.GetOwn(propName); exists {
				*dest = prop
				return true, InterpretOK, *dest
			}
		}

		// Check function prototype methods
		if FunctionPrototype != nil {
			if method, exists := FunctionPrototype.GetOwn(propName); exists {
				*dest = createBoundMethod(*objVal, method)
				return true, InterpretOK, *dest
			}
		}

		*dest = Undefined
		return true, InterpretOK, *dest
	}

	// General property lookup
	if !objVal.IsObject() {
		//frame.ip = ip // what is this for???
		// Check for null/undefined specifically for a better error message
		switch objVal.Type() {
		case TypeNull, TypeUndefined:
			status := vm.runtimeError("Cannot read property '%s' of %s", propName, objVal.TypeName())
			return false, status, Undefined
		default:
			// Generic error for other non-object types
			status := vm.runtimeError("Cannot access property '%s' on non-object type '%s'", propName, objVal.TypeName())
			return false, status, Undefined
		}
	}

	// --- INLINE CACHE CHECK (PlainObjects only for now) ---
	if objVal.Type() == TypeObject {
		po := AsPlainObject(*objVal)

		// Try cache lookup first
		if offset, hit := cache.lookupInCache(po.shape); hit {
			// Cache hit! Use cached offset directly (fast path)
			vm.cacheStats.totalHits++
			switch cache.state {
			case CacheStateMonomorphic:
				vm.cacheStats.monomorphicHits++
			case CacheStatePolymorphic:
				vm.cacheStats.polymorphicHits++
			case CacheStateMegamorphic:
				vm.cacheStats.megamorphicHits++
			}

			if offset < len(po.properties) {
				result := po.properties[offset]
				*dest = result
				return true, InterpretOK, *dest
			}
			// Cached offset is out of bounds - cache is stale, fall through to slow path
		}

		// Cache miss - do slow path lookup and update cache
		vm.cacheStats.totalMisses++
		// Use prototype-aware Get instead of GetOwn
		if fv, ok := po.Get(propName); ok {
			*dest = fv
			// Update cache only if property was found on the object itself (not prototype)
			if _, ownExists := po.GetOwn(propName); ownExists {
				// Property exists on the object itself, cache it
				for _, field := range po.shape.fields {
					if field.name == propName {
						cache.updateCache(po.shape, field.offset)
						break
					}
				}
			}
			// TODO: Implement prototype chain caching for inherited properties
		} else {
			*dest = Undefined
			// Don't cache undefined lookups for now
		}
		return true, InterpretOK, *dest
	}

	// --- Fallback for DictObject (no caching) ---
	// Dispatch to PlainObject or DictObject lookup
	switch objVal.Type() {
	case TypeDictObject:
		dict := AsDictObject(*objVal)
		// Use prototype-aware Get instead of GetOwn
		if fv, ok := dict.Get(propName); ok {
			*dest = fv
		} else {
			*dest = Undefined
		}
	default:
		// PlainObject or other object types (should not reach here due to continue above)
		po := AsPlainObject(*objVal)
		// Use prototype-aware Get instead of GetOwn
		if fv, ok := po.Get(propName); ok {
			*dest = fv
		} else {
			*dest = Undefined
		}
	}

	return true, InterpretOK, *dest
}

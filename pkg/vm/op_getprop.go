package vm

func (vm *VM) opGetProp(ip int, objVal *Value, propName string, dest *Value) (bool, InterpretResult, Value) {
	// Generate cache key
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

	// 1. Special properties (.length, etc.)
	if result, handled := vm.handleSpecialProperties(*objVal, propName); handled {
		*dest = result
		return true, InterpretOK, *dest
	}

	// 2. Primitive prototype methods (String.prototype, Array.prototype)
	if result, handled := vm.handlePrimitiveMethod(*objVal, propName); handled {
		*dest = result
		return true, InterpretOK, *dest
	}

	// 3. NativeFunctionWithProps (like String.fromCharCode)
	if objVal.Type() == TypeNativeFunctionWithProps {
		nativeFnWithProps := objVal.AsNativeFunctionWithProps()
		if prop, exists := nativeFnWithProps.Properties.GetOwn(propName); exists {
			*dest = prop
			return true, InterpretOK, *dest
		}
	}

	// 4. Functions, Closures, Native Functions, and Bound Functions (unified handling)
	if objVal.Type() == TypeFunction || objVal.Type() == TypeClosure || objVal.Type() == TypeBoundFunction || objVal.Type() == TypeNativeFunction {
		if result, handled := vm.handleCallableProperty(*objVal, propName); handled {
			*dest = result
			return true, InterpretOK, *dest
		}
	}

	// 5. General object property lookup
	if !objVal.IsObject() {
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

	// 6. PlainObject with inline cache
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

		// Cache miss - do slow path lookup
		vm.cacheStats.totalMisses++

		// Use enhanced property resolution with prototype caching
		if result, found := vm.resolvePropertyWithCache(*objVal, propName, cache, cacheKey); found {
			*dest = result

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

			return true, InterpretOK, *dest
		}

		*dest = Undefined
		return true, InterpretOK, *dest
	}

	// 7. DictObject fallback (no caching)
	if objVal.Type() == TypeDictObject {
		dict := AsDictObject(*objVal)
		// Use prototype-aware Get instead of GetOwn
		if fv, ok := dict.Get(propName); ok {
			*dest = fv
		} else {
			*dest = Undefined
		}
		return true, InterpretOK, *dest
	}

	// 8. Array objects (after special properties are handled)
	if objVal.Type() == TypeArray {
		// Arrays don't have additional own properties beyond special ones
		*dest = Undefined
		return true, InterpretOK, *dest
	}

	// 9. RegExp objects (after special properties are handled)
	if objVal.Type() == TypeRegExp {
		// RegExp objects don't have additional own properties beyond special ones
		*dest = Undefined
		return true, InterpretOK, *dest
	}

	// Shouldn't reach here, but handle as undefined
	*dest = Undefined
	return true, InterpretOK, *dest
}

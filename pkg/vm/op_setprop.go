package vm

func (vm *VM) opSetProp(ip int, objVal *Value, propName string, valueToSet *Value) (bool, InterpretResult, Value) {

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

	// Handle property setting on functions
	if objVal.Type() == TypeFunction {
		fn := AsFunction(*objVal)

		// Ensure Properties object exists
		if fn.Properties == nil {
			fn.Properties = NewObject(Undefined).AsPlainObject()
		}

		// Special handling for "prototype" property - ensure it exists first
		if propName == "prototype" {
			// If setting prototype to a new value, just set it
			fn.Properties.SetOwn("prototype", *valueToSet)
		} else {
			// For other properties, just set them
			fn.Properties.SetOwn(propName, *valueToSet)
		}
		return true, InterpretOK, *valueToSet
	}

	// Handle property setting on closures (delegate to underlying function)
	if objVal.Type() == TypeClosure {
		closure := AsClosure(*objVal)
		fn := closure.Fn

		// Ensure Properties object exists
		if fn.Properties == nil {
			fn.Properties = NewObject(Undefined).AsPlainObject()
		}

		// Special handling for "prototype" property
		if propName == "prototype" {
			fn.Properties.SetOwn("prototype", *valueToSet)
		} else {
			// For other properties, just set them
			fn.Properties.SetOwn(propName, *valueToSet)
		}
		return true, InterpretOK, *valueToSet
	}

	// Check if the base is actually an object
	if !objVal.IsObject() {
		//frame.ip = ip
		// Error setting property on non-object
		status := vm.runtimeError("Cannot set property '%s' on non-object type '%s'", propName, objVal.TypeName())
		return false, status, Undefined
	}

	// --- INLINE CACHE CHECK FOR PROPERTY WRITES (PlainObjects only) ---
	if objVal.Type() == TypeObject {
		po := AsPlainObject(*objVal)

		// Try cache lookup for existing property write
		if offset, hit := cache.lookupInCache(po.shape); hit {
			// Cache hit! Check if this is an existing property update (fast path)
			vm.cacheStats.totalHits++
			switch cache.state {
			case CacheStateMonomorphic:
				vm.cacheStats.monomorphicHits++
			case CacheStatePolymorphic:
				vm.cacheStats.polymorphicHits++
			case CacheStateMegamorphic:
				vm.cacheStats.megamorphicHits++
			}

			// Check if property exists in current shape
			for _, field := range po.shape.fields {
				if field.name == propName && field.offset == offset {
					// Existing property - fast update path
					if offset < len(po.properties) {
						po.properties[offset] = *valueToSet
						return true, InterpretOK, *valueToSet
					}
					break
				}
			}
			// Cache was stale or property layout changed, fall through to slow path
		}

		// Cache miss or new property
		vm.cacheStats.totalMisses++
		
		// Normal property setting
		originalShape := po.shape
		po.SetOwn(propName, *valueToSet)

		// Update cache if shape didn't change (existing property)
		// or if shape changed (new property added)
		for _, field := range po.shape.fields {
			if field.name == propName {
				cache.updateCache(po.shape, field.offset)
				break
			}
		}

		// If shape changed significantly, we might want to invalidate related caches
		// This is a trade-off between cache accuracy and performance
		if originalShape != po.shape {
			// Shape transition occurred - could invalidate other caches
			// For now, just update this cache
		}
		return true, InterpretOK, *valueToSet
	}

	// --- Fallback for DictObject (no caching) ---
	// Set property on DictObject or PlainObject
	switch objVal.Type() {
	case TypeDictObject:
		d := AsDictObject(*objVal)
		d.SetOwn(propName, *valueToSet)
	default:
		po := AsPlainObject(*objVal)
		po.SetOwn(propName, *valueToSet)
	}

	return true, InterpretOK, *valueToSet
}

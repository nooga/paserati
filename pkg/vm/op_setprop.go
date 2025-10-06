package vm

import "fmt"

const debugOpSetProp = false

func (vm *VM) opSetProp(ip int, objVal *Value, propName string, valueToSet *Value) (bool, InterpretResult, Value) {

	// Handle Proxy objects first
	if objVal.Type() == TypeProxy {
		proxy := objVal.AsProxy()
		if proxy.Revoked {
			// Proxy is revoked, throw TypeError
			var excVal Value
			if typeErrCtor, ok := vm.GetGlobal("TypeError"); ok {
				if res, callErr := vm.Call(typeErrCtor, Undefined, []Value{NewString("Cannot set property on a revoked Proxy")}); callErr == nil {
					excVal = res
				}
			}
			if excVal.Type() == 0 {
				eo := NewObject(vm.ErrorPrototype).AsPlainObject()
				eo.SetOwn("name", NewString("TypeError"))
				eo.SetOwn("message", NewString("Cannot set property on a revoked Proxy"))
				excVal = NewValueFromPlainObject(eo)
			}
			vm.throwException(excVal)
			return false, InterpretRuntimeError, Undefined
		}

		// Check if handler has a set trap
		setTrap, ok := proxy.handler.AsPlainObject().GetOwn("set")
		if ok {
			// Validate trap is callable
			if !setTrap.IsCallable() {
				var excVal Value
				if typeErrCtor, ok := vm.GetGlobal("TypeError"); ok {
					if res, callErr := vm.Call(typeErrCtor, Undefined, []Value{NewString("'set' on proxy: trap is not a function")}); callErr == nil {
						excVal = res
					}
				}
				if excVal.Type() == 0 {
					eo := NewObject(vm.ErrorPrototype).AsPlainObject()
					eo.SetOwn("name", NewString("TypeError"))
					eo.SetOwn("message", NewString("'set' on proxy: trap is not a function"))
					excVal = NewValueFromPlainObject(eo)
				}
				vm.throwException(excVal)
				return false, InterpretRuntimeError, Undefined
			}

			// Call the set trap: handler.set(target, propertyKey, value, receiver)
			trapArgs := []Value{proxy.target, NewString(propName), *valueToSet, *objVal}
			result, err := vm.Call(setTrap, proxy.handler, trapArgs)
			if err != nil {
				if ee, ok := err.(ExceptionError); ok {
					vm.throwException(ee.GetExceptionValue())
					return false, InterpretRuntimeError, Undefined
				}
				// Wrap non-exception Go error
				var excVal Value
				if errCtor, ok := vm.GetGlobal("Error"); ok {
					if res, callErr := vm.Call(errCtor, Undefined, []Value{NewString(err.Error())}); callErr == nil {
						excVal = res
					} else {
						eo := NewObject(vm.ErrorPrototype).AsPlainObject()
						eo.SetOwn("name", NewString("Error"))
						eo.SetOwn("message", NewString(err.Error()))
						excVal = NewValueFromPlainObject(eo)
					}
				} else {
					eo := NewObject(vm.ErrorPrototype).AsPlainObject()
					eo.SetOwn("name", NewString("Error"))
					eo.SetOwn("message", NewString(err.Error()))
					excVal = NewValueFromPlainObject(eo)
				}
				vm.throwException(excVal)
				return false, InterpretRuntimeError, Undefined
			}
			return true, InterpretOK, result
		} else {
			// No set trap, fallback to target - implement directly to avoid recursion
			target := proxy.target
			if target.Type() == TypeObject {
				po := target.AsPlainObject()
				// Check for accessor first
				if _, s, _, _, ok := po.GetOwnAccessor(propName); ok {
					if s.Type() != TypeUndefined {
						_, err := vm.prepareMethodCall(s, target, []Value{*valueToSet}, 0, vm.frames[vm.frameCount-1].registers, ip)
						if err != nil {
							if ee, ok := err.(ExceptionError); ok {
								vm.throwException(ee.GetExceptionValue())
								return false, InterpretRuntimeError, Undefined
							}
							var excVal Value
							if errCtor, ok := vm.GetGlobal("Error"); ok {
								if res, callErr := vm.Call(errCtor, Undefined, []Value{NewString(err.Error())}); callErr == nil {
									excVal = res
								} else {
									eo := NewObject(vm.ErrorPrototype).AsPlainObject()
									eo.SetOwn("name", NewString("Error"))
									eo.SetOwn("message", NewString(err.Error()))
									excVal = NewValueFromPlainObject(eo)
								}
							} else {
								eo := NewObject(vm.ErrorPrototype).AsPlainObject()
								eo.SetOwn("name", NewString("Error"))
								eo.SetOwn("message", NewString(err.Error()))
								excVal = NewValueFromPlainObject(eo)
							}
							vm.throwException(excVal)
							return false, InterpretRuntimeError, Undefined
						}
						return true, InterpretOK, *valueToSet
					}
				}
				// Data property
				po.SetOwn(propName, *valueToSet)
				return true, InterpretOK, *valueToSet
			} else if target.Type() == TypeDictObject {
				dict := target.AsDictObject()
				dict.SetOwn(propName, *valueToSet)
				return true, InterpretOK, *valueToSet
			} else {
				// For other types, just return the value
				return true, InterpretOK, *valueToSet
			}
		}
	}

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

	// Handle property setting on function-like values
	switch objVal.Type() {
	case TypeFunction:
		fn := AsFunction(*objVal)
		if fn.Properties == nil {
			fn.Properties = NewObject(Undefined).AsPlainObject()
		}
		if propName == "prototype" {
			fn.Properties.SetOwn("prototype", *valueToSet)
		} else {
			fn.Properties.SetOwn(propName, *valueToSet)
		}
		return true, InterpretOK, *valueToSet
	case TypeClosure:
		closure := AsClosure(*objVal)
		fn := closure.Fn
		if fn.Properties == nil {
			fn.Properties = NewObject(Undefined).AsPlainObject()
		}
		if propName == "prototype" {
			fn.Properties.SetOwn("prototype", *valueToSet)
		} else {
			fn.Properties.SetOwn(propName, *valueToSet)
		}
		return true, InterpretOK, *valueToSet
	case TypeNativeFunctionWithProps:
		nfp := objVal.AsNativeFunctionWithProps()
		if nfp != nil && nfp.Properties != nil {
			nfp.Properties.SetOwn(propName, *valueToSet)
			return true, InterpretOK, *valueToSet
		}
		// If somehow missing props container, create and retry
		if nfp != nil && nfp.Properties == nil {
			nfp.Properties = NewObject(Undefined).AsPlainObject()
			nfp.Properties.SetOwn(propName, *valueToSet)
			return true, InterpretOK, *valueToSet
		}
	case TypeNativeFunction:
		// Promote plain native function to one that supports properties
		nf := objVal.AsNativeFunction()
		if nf != nil {
			promoted := NewNativeFunctionWithProps(nf.Arity, nf.Variadic, nf.Name, nf.Fn)
			*objVal = promoted
			if nfp := promoted.AsNativeFunctionWithProps(); nfp != nil {
				nfp.Properties.SetOwn(propName, *valueToSet)
				return true, InterpretOK, *valueToSet
			}
		}
	}

	// (Bound/async native functions currently do not support own props; extend if needed)

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

		// Try cache lookup for existing property write (check accessor/writable flags)
		if entry, hit := cache.lookupEntry(po.shape); hit {
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

			// Accessor? Defer to slow path to call setter
			if !entry.isAccessor && entry.writable {
				if entry.offset < len(po.properties) {
					po.properties[entry.offset] = *valueToSet
					return true, InterpretOK, *valueToSet
				}
			}
			// Cache was stale or property layout changed, fall through to slow path
		}

		// Cache miss or new property
		vm.cacheStats.totalMisses++

		// Normal property setting with accessor awareness
		originalShape := po.shape
		// Check for accessor in prototype chain (not just own properties)
		if debugOpSetProp {
			fmt.Printf("[DEBUG opSetProp setter check] propName=%q\n", propName)
		}

		// Walk prototype chain looking for setter
		current := po
		for current != nil {
			if _, s, _, _, ok := current.GetOwnAccessor(propName); ok {
				if debugOpSetProp {
					fmt.Printf("[DEBUG opSetProp] GetOwnAccessor found accessor for %q, setter type=%s\n", propName, s.Type().String())
				}
				if s.Type() != TypeUndefined {
					if debugOpSetProp {
						fmt.Printf("[DEBUG opSetProp] Invoking setter for %q with value=%v\n", propName, *valueToSet)
					}
					// Call setter with this=objVal (original object, not prototype)
					_, err := vm.Call(s, *objVal, []Value{*valueToSet})
					if err != nil {
						// Propagate as VM exception
						if ee, ok := err.(ExceptionError); ok {
							vm.throwException(ee.GetExceptionValue())
							return false, InterpretRuntimeError, Undefined
						}
						// Wrap non-exception Go error into a proper JS Error instance and throw
						var excVal Value
						if errCtor, ok := vm.GetGlobal("Error"); ok {
							if res, callErr := vm.Call(errCtor, Undefined, []Value{NewString(err.Error())}); callErr == nil {
								excVal = res
							} else {
								eo := NewObject(vm.ErrorPrototype).AsPlainObject()
								eo.SetOwn("name", NewString("Error"))
								eo.SetOwn("message", NewString(err.Error()))
								excVal = NewValueFromPlainObject(eo)
							}
						} else {
							eo := NewObject(vm.ErrorPrototype).AsPlainObject()
							eo.SetOwn("name", NewString("Error"))
							eo.SetOwn("message", NewString(err.Error()))
							excVal = NewValueFromPlainObject(eo)
						}
						vm.throwException(excVal)
						return false, InterpretRuntimeError, Undefined
					}
					// If setter handled, return original value per JS semantics (assignment expr yields RHS)
					if debugOpSetProp {
						fmt.Printf("[DEBUG opSetProp] Setter invoked successfully\n")
					}
					return true, InterpretOK, *valueToSet
				}
				// No setter: ignore in non-strict
				if debugOpSetProp {
					fmt.Printf("[DEBUG opSetProp] Setter is undefined, ignoring\n")
				}
				return true, InterpretOK, *valueToSet
			}

			// Move up the prototype chain
			protoVal := current.GetPrototype()
			if !protoVal.IsObject() {
				break
			}
			current = protoVal.AsPlainObject()
		}
		// Data property path
		if debugOpSetProp {
			fmt.Printf("[DEBUG opSetProp] No accessor found, using data property path\n")
		}
		po.SetOwn(propName, *valueToSet)

		// Update cache if shape didn't change (existing property)
		// or if shape changed (new property added)
		for _, field := range po.shape.fields {
			if field.name == propName {
				cache.updateCache(po.shape, field.offset, field.isAccessor, field.writable)
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

// opSetPropSymbol handles setting a symbol-keyed property on an object with IC support.
func (vm *VM) opSetPropSymbol(ip int, objVal *Value, symKey Value, valueToSet *Value) (bool, InterpretResult, Value) {
	// Handle Proxy objects first
	if objVal.Type() == TypeProxy {
		proxy := objVal.AsProxy()
		if proxy.Revoked {
			// Proxy is revoked, throw TypeError
			var excVal Value
			if typeErrCtor, ok := vm.GetGlobal("TypeError"); ok {
				if res, callErr := vm.Call(typeErrCtor, Undefined, []Value{NewString("Cannot set property on a revoked Proxy")}); callErr == nil {
					excVal = res
				}
			}
			if excVal.Type() == 0 {
				eo := NewObject(vm.ErrorPrototype).AsPlainObject()
				eo.SetOwn("name", NewString("TypeError"))
				eo.SetOwn("message", NewString("Cannot set property on a revoked Proxy"))
				excVal = NewValueFromPlainObject(eo)
			}
			vm.throwException(excVal)
			return false, InterpretRuntimeError, Undefined
		}

		// Check if handler has a set trap
		setTrap, ok := proxy.handler.AsPlainObject().GetOwn("set")
		if ok && setTrap.IsCallable() {
			// Call the set trap: handler.set(target, propertyKey, value, receiver)
			trapArgs := []Value{proxy.target, symKey, *valueToSet, *objVal}
			result, err := vm.Call(setTrap, proxy.handler, trapArgs)
			if err != nil {
				if ee, ok := err.(ExceptionError); ok {
					vm.throwException(ee.GetExceptionValue())
					return false, InterpretRuntimeError, Undefined
				}
				// Wrap non-exception Go error
				var excVal Value
				if errCtor, ok := vm.GetGlobal("Error"); ok {
					if res, callErr := vm.Call(errCtor, Undefined, []Value{NewString(err.Error())}); callErr == nil {
						excVal = res
					} else {
						eo := NewObject(vm.ErrorPrototype).AsPlainObject()
						eo.SetOwn("name", NewString("Error"))
						eo.SetOwn("message", NewString(err.Error()))
						excVal = NewValueFromPlainObject(eo)
					}
				} else {
					eo := NewObject(vm.ErrorPrototype).AsPlainObject()
					eo.SetOwn("name", NewString("Error"))
					eo.SetOwn("message", NewString(err.Error()))
					excVal = NewValueFromPlainObject(eo)
				}
				vm.throwException(excVal)
				return false, InterpretRuntimeError, Undefined
			}
			return true, InterpretOK, result
		} else {
			// No set trap, fallback to target - implement directly to avoid recursion
			target := proxy.target
			if target.Type() == TypeObject {
				po := target.AsPlainObject()
				key := NewSymbolKey(symKey)
				// Check for accessor first
				if _, s, _, _, ok := po.GetOwnAccessorByKey(key); ok {
					if s.Type() != TypeUndefined {
						_, err := vm.Call(s, target, []Value{*valueToSet})
						if err != nil {
							if ee, ok := err.(ExceptionError); ok {
								vm.throwException(ee.GetExceptionValue())
								return false, InterpretRuntimeError, Undefined
							}
							var excVal Value
							if errCtor, ok := vm.GetGlobal("Error"); ok {
								if res, callErr := vm.Call(errCtor, Undefined, []Value{NewString(err.Error())}); callErr == nil {
									excVal = res
								} else {
									eo := NewObject(vm.ErrorPrototype).AsPlainObject()
									eo.SetOwn("name", NewString("Error"))
									eo.SetOwn("message", NewString(err.Error()))
									excVal = NewValueFromPlainObject(eo)
								}
							} else {
								eo := NewObject(vm.ErrorPrototype).AsPlainObject()
								eo.SetOwn("name", NewString("Error"))
								eo.SetOwn("message", NewString(err.Error()))
								excVal = NewValueFromPlainObject(eo)
							}
							vm.throwException(excVal)
							return false, InterpretRuntimeError, Undefined
						}
						return true, InterpretOK, *valueToSet
					}
				}
				// Data property
				po.DefineOwnPropertyByKey(key, *valueToSet, nil, nil, nil)
				return true, InterpretOK, *valueToSet
			} else {
				// For other types, just return the value
				return true, InterpretOK, *valueToSet
			}
		}
	}

	// Only PlainObject supports symbol keys for now
	if objVal.Type() != TypeObject {
		// DictObject or others: ignore symbol set (non-strict semantics)
		return true, InterpretOK, *valueToSet
	}

	po := AsPlainObject(*objVal)

	// Per-site cache keyed by symbol identity
	cacheKey := generateSymbolCacheKey(ip, symKey)
	cache, exists := vm.propCache[cacheKey]
	if !exists {
		cache = &PropInlineCache{state: CacheStateUninitialized}
		vm.propCache[cacheKey] = cache
	}

	// Fast path: cache hit
	if entry, hit := cache.lookupEntry(po.shape); hit {
		vm.cacheStats.totalHits++
		switch cache.state {
		case CacheStateMonomorphic:
			vm.cacheStats.monomorphicHits++
		case CacheStatePolymorphic:
			vm.cacheStats.polymorphicHits++
		case CacheStateMegamorphic:
			vm.cacheStats.megamorphicHits++
		}

		if entry.isAccessor {
			// Accessor: call setter if present
			if _, s, _, _, ok := po.GetOwnAccessorByKey(NewSymbolKey(symKey)); ok && s.Type() != TypeUndefined {
				_, err := vm.Call(s, *objVal, []Value{*valueToSet})
				if err != nil {
					if ee, ok := err.(ExceptionError); ok {
						vm.throwException(ee.GetExceptionValue())
						return false, InterpretRuntimeError, Undefined
					}
					var excVal Value
					if errCtor, ok := vm.GetGlobal("Error"); ok {
						if res, callErr := vm.Call(errCtor, Undefined, []Value{NewString(err.Error())}); callErr == nil {
							excVal = res
						} else {
							eo := NewObject(vm.ErrorPrototype).AsPlainObject()
							eo.SetOwn("name", NewString("Error"))
							eo.SetOwn("message", NewString(err.Error()))
							excVal = NewValueFromPlainObject(eo)
						}
					} else {
						eo := NewObject(vm.ErrorPrototype).AsPlainObject()
						eo.SetOwn("name", NewString("Error"))
						eo.SetOwn("message", NewString(err.Error()))
						excVal = NewValueFromPlainObject(eo)
					}
					vm.throwException(excVal)
					return false, InterpretRuntimeError, Undefined
				}
				return true, InterpretOK, *valueToSet
			}
			// No setter: ignore in non-strict
			return true, InterpretOK, *valueToSet
		}

		if entry.writable && entry.offset < len(po.properties) {
			po.properties[entry.offset] = *valueToSet
			return true, InterpretOK, *valueToSet
		}
		// Not writable or stale: fall through to slow path
	}

	// Slow path miss
	vm.cacheStats.totalMisses++

	// Accessor own setter path
	if _, s, _, _, ok := po.GetOwnAccessorByKey(NewSymbolKey(symKey)); ok {
		if s.Type() != TypeUndefined {
			_, err := vm.Call(s, *objVal, []Value{*valueToSet})
			if err != nil {
				if ee, ok := err.(ExceptionError); ok {
					vm.throwException(ee.GetExceptionValue())
					return false, InterpretRuntimeError, Undefined
				}
				var excVal Value
				if errCtor, ok := vm.GetGlobal("Error"); ok {
					if res, callErr := vm.Call(errCtor, Undefined, []Value{NewString(err.Error())}); callErr == nil {
						excVal = res
					} else {
						eo := NewObject(vm.ErrorPrototype).AsPlainObject()
						eo.SetOwn("name", NewString("Error"))
						eo.SetOwn("message", NewString(err.Error()))
						excVal = NewValueFromPlainObject(eo)
					}
				} else {
					eo := NewObject(vm.ErrorPrototype).AsPlainObject()
					eo.SetOwn("name", NewString("Error"))
					eo.SetOwn("message", NewString(err.Error()))
					excVal = NewValueFromPlainObject(eo)
				}
				vm.throwException(excVal)
				return false, InterpretRuntimeError, Undefined
			}
			return true, InterpretOK, *valueToSet
		}
		// No setter: ignore
		return true, InterpretOK, *valueToSet
	}

	// Try to find existing symbol field and update
	updated := false
	for _, f := range po.shape.fields {
		if f.keyKind == KeyKindSymbol && f.symbolVal.obj == symKey.obj {
			if f.writable && f.offset < len(po.properties) {
				po.properties[f.offset] = *valueToSet
			}
			// Update cache with flags
			cache.updateCache(po.shape, f.offset, f.isAccessor, f.writable)
			updated = true
			break
		}
	}
	if updated {
		return true, InterpretOK, *valueToSet
	}

	// Define new data property by symbol key (defaults false unless specified elsewhere)
	po.DefineOwnPropertyByKey(NewSymbolKey(symKey), *valueToSet, nil, nil, nil)
	// Find new field to cache
	for _, f := range po.shape.fields {
		if f.keyKind == KeyKindSymbol && f.symbolVal.obj == symKey.obj {
			cache.updateCache(po.shape, f.offset, f.isAccessor, f.writable)
			break
		}
	}
	return true, InterpretOK, *valueToSet
}

package vm

import "fmt"

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

	if debugVM {
		fmt.Printf("[opGetProp] ip=%d obj=%s(%s) prop=%q\n", ip, objVal.Inspect(), objVal.TypeName(), propName)
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
			if debugVM {
				if propName == "name" || propName == "constructor" {
					if debugVM {
						fmt.Printf("[DBG opGetProp] '%s' hit own property on NativeFunctionWithProps: %s (%s)\n", propName, prop.Inspect(), prop.TypeName())
					}
				}
			}
			*dest = prop
			return true, InterpretOK, *dest
		}
	}

	// 4. Functions, Closures, Native Functions, Native Functions with Props, Async Native Functions, and Bound Functions (unified handling)
	if objVal.Type() == TypeFunction || objVal.Type() == TypeClosure || objVal.Type() == TypeBoundFunction || objVal.Type() == TypeNativeFunction || objVal.Type() == TypeNativeFunctionWithProps || objVal.Type() == TypeAsyncNativeFunction {
		if result, handled := vm.handleCallableProperty(*objVal, propName); handled {
			if debugVM {
				if propName == "name" || propName == "constructor" {
					if debugVM {
						fmt.Printf("[DBG opGetProp] '%s' via handleCallableProperty -> %s (%s)\n", propName, result.Inspect(), result.TypeName())
					}
				}
			}
			*dest = result
			return true, InterpretOK, *dest
		}
	}

	// 5. Arguments object property lookup (delegate to Object.prototype)
	if objVal.Type() == TypeArguments {
		// Arguments objects should inherit from Object.prototype
		if vm.ObjectPrototype.Type() == TypeObject {
			objProto := vm.ObjectPrototype.AsPlainObject()
			if method, exists := objProto.GetOwn(propName); exists {
				*dest = method
				return true, InterpretOK, *dest
			}
		}
		// Property not found on Object.prototype
		*dest = Undefined
		return true, InterpretOK, *dest
	}

	// 6. General object property lookup
	if !objVal.IsObject() {
		// Check for null/undefined specifically for a better error message
		switch objVal.Type() {
		case TypeNull, TypeUndefined:
			// Throw JS TypeError: Cannot read property 'X' of null/undefined
			var excVal Value
			if typeErrCtor, ok := vm.GetGlobal("TypeError"); ok {
				if res, callErr := vm.Call(typeErrCtor, Undefined, []Value{NewString(fmt.Sprintf("Cannot read property '%s' of %s", propName, objVal.TypeName()))}); callErr == nil {
					excVal = res
				}
			}
			if excVal.Type() == 0 {
				eo := NewObject(vm.ErrorPrototype).AsPlainObject()
				eo.SetOwn("name", NewString("TypeError"))
				eo.SetOwn("message", NewString(fmt.Sprintf("Cannot read property '%s' of %s", propName, objVal.TypeName())))
				excVal = NewValueFromPlainObject(eo)
			}
			vm.throwException(excVal)
			return false, InterpretRuntimeError, Undefined
		default:
			// Generic error for other non-object types -> TypeError
			if debugVM && (propName == "value" || propName == "next") {
				if debugVM {
					fmt.Printf("[DBG opGetProp] Trap '%s' on non-object %s value=%s\n", propName, objVal.TypeName(), objVal.Inspect())
				}
				if vm.frameCount > 0 {
					fr := &vm.frames[vm.frameCount-1]
					topN := 0
					if len(fr.registers) < topN {
						topN = len(fr.registers)
					}
					for i := 0; i < topN; i++ {
						fmt.Printf("    [R%d]=%s(%s)\n", i, fr.registers[i].Inspect(), fr.registers[i].TypeName())
					}
				}
			} else if debugVM {
				fmt.Printf("[DBG opGetProp] ERROR: '%s' on non-object %s value=%s\n", propName, objVal.TypeName(), objVal.Inspect())
			}
			var excVal Value
			if typeErrCtor, ok := vm.GetGlobal("TypeError"); ok {
				if res, callErr := vm.Call(typeErrCtor, Undefined, []Value{NewString(fmt.Sprintf("Cannot access property '%s' on non-object type '%s'", propName, objVal.TypeName()))}); callErr == nil {
					excVal = res
				}
			}
			if excVal.Type() == 0 {
				eo := NewObject(vm.ErrorPrototype).AsPlainObject()
				eo.SetOwn("name", NewString("TypeError"))
				eo.SetOwn("message", NewString(fmt.Sprintf("Cannot access property '%s' on non-object type '%s'", propName, objVal.TypeName())))
				excVal = NewValueFromPlainObject(eo)
			}
			vm.throwException(excVal)
			return false, InterpretRuntimeError, Undefined
		}
	}

	// Additional debug: when asking for 'constructor' on an object, show prototype's name if present
	if false && propName == "constructor" && objVal.Type() == TypeObject {
		po := AsPlainObject(*objVal)
		proto := po.GetPrototype()
		protoName := "<no name>"
		if proto.IsObject() {
			if n, ok := proto.AsPlainObject().GetOwn("name"); ok {
				protoName = n.ToString()
			}
		}
		_ = protoName
	}

	// 6. PlainObject with inline cache
	if objVal.Type() == TypeObject {
		po := AsPlainObject(*objVal)

		// Try cache lookup first (full entry): handle own and proto hits
		if entry, hit := cache.lookupEntry(po.shape); hit {
			if debugVM {
				fmt.Printf("[opGetProp] IC hit state=%d isProto=%v accessor=%v offset=%d\n", cache.state, entry.isProto, entry.isAccessor, entry.offset)
			}
			vm.cacheStats.totalHits++
			switch cache.state {
			case CacheStateMonomorphic:
				vm.cacheStats.monomorphicHits++
			case CacheStatePolymorphic:
				vm.cacheStats.polymorphicHits++
			case CacheStateMegamorphic:
				vm.cacheStats.megamorphicHits++
			}
			if !entry.isProto {
				if entry.isAccessor {
					// Own accessor fast path: call getter with this=obj
					if g, _, _, _, ok := po.GetOwnAccessor(propName); ok && g.Type() != TypeUndefined {
						// Use unified Call to execute getter synchronously
						res, err := vm.Call(g, *objVal, nil)
						if err != nil {
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
						*dest = res
						return true, InterpretOK, *dest
					}
					// No getter defined: undefined per spec
					*dest = Undefined
					return true, InterpretOK, *dest
				} else {
					if entry.offset < len(po.properties) {
						result := po.properties[entry.offset]
						*dest = result
						return true, InterpretOK, *dest
					}
				}
			} else {
				// Walk protoDepth and validate holder shape/version
				current := po
				for i := int8(0); i < entry.protoDepth && current != nil; i++ {
					pv := current.GetPrototype()
					if !pv.IsObject() {
						current = nil
						break
					}
					current = pv.AsPlainObject()
				}
				if current != nil && current.shape == entry.holderShape && current.shape.version == entry.holderVersion {
					if entry.isAccessor {
						if g, _, _, _, ok := current.GetOwnAccessor(propName); ok && g.Type() != TypeUndefined {
							res, err := vm.Call(g, *objVal, nil)
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
							*dest = res
							return true, InterpretOK, *dest
						}
						*dest = Undefined
						return true, InterpretOK, *dest
					} else if entry.offset < len(current.properties) {
						*dest = current.properties[entry.offset]
						return true, InterpretOK, *dest
					}
				}
				// else stale; fall through
			}
		}

		// Cache miss - do slow path lookup
		if debugVM {
			fmt.Printf("[opGetProp] IC miss, resolving slow path for %q\n", propName)
		}
		vm.cacheStats.totalMisses++

		// Use enhanced property resolution with prototype caching and metadata
		if holder, offset, isAccessor, found := vm.resolvePropertyMeta(*objVal, propName, cache, cacheKey); found {
			if isAccessor {
				if g, _, _, _, ok := holder.GetOwnAccessor(propName); ok && g.Type() != TypeUndefined {
					res, err := vm.Call(g, *objVal, nil)
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
					*dest = res
				} else {
					*dest = Undefined
				}
			} else {
				*dest = holder.properties[offset]
			}
			if propName == "next" {
				if debugVM {
					fmt.Printf("[DBG opGetProp] resolved 'next' via proto/cache -> %s (%s)\n", dest.Inspect(), dest.TypeName())
				}
			}

			// Update cache flags for direct own properties
			if holder == po {
				for _, field := range po.shape.fields {
					if field.name == propName {
						cache.updateCache(po.shape, field.offset, field.isAccessor, field.writable)
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
			if propName == "next" {
				if debugVM {
					fmt.Printf("[DBG opGetProp] (dict) resolved 'next' -> %s (%s)\n", fv.Inspect(), fv.TypeName())
				}
			}
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

	// 9. Map objects (after special properties are handled)
	if objVal.Type() == TypeMap {
		// Maps don't have additional own properties beyond special ones
		*dest = Undefined
		return true, InterpretOK, *dest
	}

	// 10. Set objects (after special properties are handled)
	if objVal.Type() == TypeSet {
		// Sets don't have additional own properties beyond special ones
		*dest = Undefined
		return true, InterpretOK, *dest
	}

	// 11. Generator objects
	if objVal.Type() == TypeGenerator {
		// Generator objects don't have additional own properties beyond special ones
		*dest = Undefined
		return true, InterpretOK, *dest
	}

	// 12. RegExp objects (after special properties are handled)
	if objVal.Type() == TypeRegExp {
		// RegExp objects don't have additional own properties beyond special ones
		*dest = Undefined
		return true, InterpretOK, *dest
	}

	// Shouldn't reach here, but handle as undefined
	*dest = Undefined
	return true, InterpretOK, *dest
}

// opGetPropSymbol handles property get where the key is a symbol Value.
func (vm *VM) opGetPropSymbol(ip int, objVal *Value, symKey Value, dest *Value) (bool, InterpretResult, Value) {
	// Prepare a per-site cache key for symbol lookups (future use)
	_ = generateSymbolCacheKey // reference to avoid unused warning if not used yet
	// cacheKey := generateSymbolCacheKey(ip, symKey)
	// Resolve a prototype-backed view for primitives
	base := *objVal
	switch base.Type() {
	case TypeString:
		// Emulate boxing: access via String.prototype
		proto := vm.StringPrototype
		if proto.IsObject() {
			po := proto.AsPlainObject()
			if v, ok := po.GetOwnByKey(NewSymbolKey(symKey)); ok {
				*dest = v
				if debugVM {
					fmt.Printf("[DBG opGetPropSymbol] String.prototype[%s] -> %s (%s)\n", symKey.AsSymbol(), v.Inspect(), v.TypeName())
				}
				return true, InterpretOK, *dest
			}
			// Walk prototype chain from String.prototype
			current := po.prototype
			for current.typ != TypeNull && current.typ != TypeUndefined {
				if current.IsObject() {
					if proto2 := current.AsPlainObject(); proto2 != nil {
						if v, ok := proto2.GetOwnByKey(NewSymbolKey(symKey)); ok {
							*dest = v
							if debugVM {
								fmt.Printf("[DBG opGetPropSymbol] String proto-chain[%s] -> %s (%s)\n", symKey.AsSymbol(), v.Inspect(), v.TypeName())
							}
							return true, InterpretOK, *dest
						}
						current = proto2.prototype
					} else if dict := current.AsDictObject(); dict != nil {
						current = dict.prototype
					} else {
						break
					}
				} else {
					break
				}
			}
		}
		// Not found on String.prototype chain for symbol key
		*dest = Undefined
		return true, InterpretOK, *dest
	case TypeArray:
		// Arrays: consult Array.prototype chain for symbol properties
		proto := vm.ArrayPrototype
		if proto.IsObject() {
			po := proto.AsPlainObject()
			if v, ok := po.GetOwnByKey(NewSymbolKey(symKey)); ok {
				*dest = v
				if debugVM {
					fmt.Printf("[DBG opGetPropSymbol] Array.prototype[%s] -> %s (%s)\n", symKey.AsSymbol(), v.Inspect(), v.TypeName())
				}
				return true, InterpretOK, *dest
			}
			current := po.prototype
			for current.typ != TypeNull && current.typ != TypeUndefined {
				if current.IsObject() {
					if proto2 := current.AsPlainObject(); proto2 != nil {
						if v, ok := proto2.GetOwnByKey(NewSymbolKey(symKey)); ok {
							*dest = v
							if debugVM {
								fmt.Printf("[DBG opGetPropSymbol] Array proto-chain[%s] -> %s (%s)\n", symKey.AsSymbol(), v.Inspect(), v.TypeName())
							}
							return true, InterpretOK, *dest
						}
						current = proto2.prototype
					} else if dict := current.AsDictObject(); dict != nil {
						current = dict.prototype
					} else {
						break
					}
				} else {
					break
				}
			}
		}
		*dest = Undefined
		return true, InterpretOK, *dest
	case TypeGenerator:
		// Generators: consult Generator.prototype chain for symbol properties
		proto := vm.GeneratorPrototype
		if proto.IsObject() {
			po := proto.AsPlainObject()
			if v, ok := po.GetOwnByKey(NewSymbolKey(symKey)); ok {
				*dest = v
				if debugVM {
					fmt.Printf("[DBG opGetPropSymbol] Generator.prototype[%s] -> %s (%s)\n", symKey.AsSymbol(), v.Inspect(), v.TypeName())
				}
				return true, InterpretOK, *dest
			}
			current := po.prototype
			for current.typ != TypeNull && current.typ != TypeUndefined {
				if current.IsObject() {
					if proto2 := current.AsPlainObject(); proto2 != nil {
						if v, ok := proto2.GetOwnByKey(NewSymbolKey(symKey)); ok {
							*dest = v
							if debugVM {
								fmt.Printf("[DBG opGetPropSymbol] Generator proto-chain[%s] -> %s (%s)\n", symKey.AsSymbol(), v.Inspect(), v.TypeName())
							}
							return true, InterpretOK, *dest
						}
						current = proto2.prototype
					} else if dict := current.AsDictObject(); dict != nil {
						current = dict.prototype
					} else {
						break
					}
				} else {
					break
				}
			}
		}
		*dest = Undefined
		return true, InterpretOK, *dest
	}

	// Map: consult Map.prototype for symbol properties (e.g., [Symbol.iterator])
	if base.Type() == TypeMap {
		proto := vm.MapPrototype
		if proto.IsObject() {
			po := proto.AsPlainObject()
			if v, ok := po.GetOwnByKey(NewSymbolKey(symKey)); ok {
				*dest = v
				return true, InterpretOK, *dest
			}
			current := po.prototype
			for current.typ != TypeNull && current.typ != TypeUndefined {
				if current.IsObject() {
					if proto2 := current.AsPlainObject(); proto2 != nil {
						if v, ok := proto2.GetOwnByKey(NewSymbolKey(symKey)); ok {
							*dest = v
							return true, InterpretOK, *dest
						}
						current = proto2.prototype
					} else if dict := current.AsDictObject(); dict != nil {
						current = dict.prototype
					} else {
						break
					}
				} else {
					break
				}
			}
		}
		*dest = Undefined
		return true, InterpretOK, *dest
	}

	// Set: consult Set.prototype for symbol properties
	if base.Type() == TypeSet {
		proto := vm.SetPrototype
		if proto.IsObject() {
			po := proto.AsPlainObject()
			if v, ok := po.GetOwnByKey(NewSymbolKey(symKey)); ok {
				*dest = v
				return true, InterpretOK, *dest
			}
			current := po.prototype
			for current.typ != TypeNull && current.typ != TypeUndefined {
				if current.IsObject() {
					if proto2 := current.AsPlainObject(); proto2 != nil {
						if v, ok := proto2.GetOwnByKey(NewSymbolKey(symKey)); ok {
							*dest = v
							return true, InterpretOK, *dest
						}
						current = proto2.prototype
					} else if dict := current.AsDictObject(); dict != nil {
						current = dict.prototype
					} else {
						break
					}
				} else {
					break
				}
			}
		}
		*dest = Undefined
		return true, InterpretOK, *dest
	}

	// PlainObject: search by symbol identity (with accessor invocation semantics)
	if base.Type() == TypeObject {
		po := AsPlainObject(base)
		key := NewSymbolKey(symKey)
		// Own accessor first
		if g, _, _, _, ok := po.GetOwnAccessorByKey(key); ok {
			if g.Type() != TypeUndefined {
				res, err := vm.Call(g, base, nil)
				if err != nil {
					if ee, ok := err.(ExceptionError); ok {
						vm.throwException(ee.GetExceptionValue())
						return false, InterpretRuntimeError, Undefined
					}
					// Wrap non-exception Go error
					var excVal Value
					if errCtor, ok := vm.GetGlobal("Error"); ok {
						if res2, callErr := vm.Call(errCtor, Undefined, []Value{NewString(err.Error())}); callErr == nil {
							excVal = res2
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
				*dest = res
				return true, InterpretOK, *dest
			}
			*dest = Undefined
			return true, InterpretOK, *dest
		}
		// Own data property
		if v, ok := po.GetOwnByKey(key); ok {
			*dest = v
			return true, InterpretOK, *dest
		}
		// Walk prototype chain searching for accessor/data
		current := po.GetPrototype()
		for current.typ != TypeNull && current.typ != TypeUndefined {
			if !current.IsObject() {
				break
			}
			if proto := current.AsPlainObject(); proto != nil {
				if g, _, _, _, ok := proto.GetOwnAccessorByKey(key); ok {
					if g.Type() != TypeUndefined {
						res, err := vm.Call(g, base, nil)
						if err != nil {
							if ee, ok := err.(ExceptionError); ok {
								vm.throwException(ee.GetExceptionValue())
								return false, InterpretRuntimeError, Undefined
							}
							var excVal Value
							if errCtor, ok := vm.GetGlobal("Error"); ok {
								if res2, callErr := vm.Call(errCtor, Undefined, []Value{NewString(err.Error())}); callErr == nil {
									excVal = res2
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
						*dest = res
						return true, InterpretOK, *dest
					}
					*dest = Undefined
					return true, InterpretOK, *dest
				}
				if v, ok := proto.GetOwnByKey(key); ok {
					*dest = v
					return true, InterpretOK, *dest
				}
				current = proto.prototype
				continue
			}
			if dict := current.AsDictObject(); dict != nil {
				current = dict.prototype
				continue
			}
			break
		}
		*dest = Undefined
		return true, InterpretOK, *dest
	}

	// DictObject: no symbol identity support yet
	*dest = Undefined
	return true, InterpretOK, *dest
}

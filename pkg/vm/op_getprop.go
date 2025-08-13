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

	// Debug trace for tricky harness props
	if debugVM {
		if propName == "name" || propName == "constructor" || propName == "value" || propName == "next" {
			if debugVM {
				fmt.Printf("[DBG opGetProp] '%s' on %s (%s)\n", propName, objVal.Inspect(), objVal.TypeName())
			}
		}
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
			status := vm.runtimeError("Cannot read property '%s' of %s", propName, objVal.TypeName())
			return false, status, Undefined
		default:
			// Generic error for other non-object types
			if debugVM && (propName == "value" || propName == "next") {
				if debugVM {
					fmt.Printf("[DBG opGetProp] Trap '%s' on non-object %s value=%s\n", propName, objVal.TypeName(), objVal.Inspect())
				}
				// Try to dump a small backtrace of registers around current frame if available
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
			} else {
				if debugVM {
					fmt.Printf("[DBG opGetProp] ERROR: '%s' on non-object %s value=%s\n", propName, objVal.TypeName(), objVal.Inspect())
				}
			}
			status := vm.runtimeError("Cannot access property '%s' on non-object type '%s'", propName, objVal.TypeName())
			return false, status, Undefined
		}
	}

	// Additional debug: when asking for 'constructor' on an object, show prototype's name if present
	if debugVM && propName == "constructor" && objVal.Type() == TypeObject {
		po := AsPlainObject(*objVal)
		proto := po.GetPrototype()
		protoName := "<no name>"
		if proto.IsObject() {
			if n, ok := proto.AsPlainObject().GetOwn("name"); ok {
				protoName = n.ToString()
			}
		}
		if debugVM {
			fmt.Printf("[DBG opGetProp] object prototype name: %s\n", protoName)
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
				if propName == "next" {
					if debugVM {
						fmt.Printf("[DBG opGetProp] resolved 'next' -> %s (%s)\n", result.Inspect(), result.TypeName())
					}
				}
				return true, InterpretOK, *dest
			}
			// Cached offset is out of bounds - cache is stale, fall through to slow path
		}

		// Cache miss - do slow path lookup
		vm.cacheStats.totalMisses++

		// Use enhanced property resolution with prototype caching
		if result, found := vm.resolvePropertyWithCache(*objVal, propName, cache, cacheKey); found {
			*dest = result
			if propName == "next" {
				if debugVM {
					fmt.Printf("[DBG opGetProp] resolved 'next' via proto/cache -> %s (%s)\n", result.Inspect(), result.TypeName())
				}
			}

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

	// PlainObject: search by symbol identity
	if base.Type() == TypeObject {
		po := AsPlainObject(base)
		if v, ok := po.GetOwnByKey(NewSymbolKey(symKey)); ok {
			*dest = v
			if debugVM {
				fmt.Printf("[DBG opGetPropSymbol] PlainObject own[%s] -> %s (%s)\n", symKey.AsSymbol(), v.Inspect(), v.TypeName())
			}
			return true, InterpretOK, *dest
		}
		// Walk prototype chain
		current := po.GetPrototype()
		for current.typ != TypeNull && current.typ != TypeUndefined {
			if current.IsObject() {
				if proto := current.AsPlainObject(); proto != nil {
					if v, ok := proto.GetOwnByKey(NewSymbolKey(symKey)); ok {
						*dest = v
						if debugVM {
							fmt.Printf("[DBG opGetPropSymbol] PlainObject proto-chain[%s] -> %s (%s)\n", symKey.AsSymbol(), v.Inspect(), v.TypeName())
						}
						return true, InterpretOK, *dest
					}
					current = proto.prototype
				} else if dict := current.AsDictObject(); dict != nil {
					current = dict.prototype
				} else {
					break
				}
			} else {
				break
			}
		}
		*dest = Undefined
		return true, InterpretOK, *dest
	}

	// DictObject: no symbol identity support yet
	*dest = Undefined
	return true, InterpretOK, *dest
}

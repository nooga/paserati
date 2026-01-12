package vm

import "fmt"

func (vm *VM) opGetProp(frame *CallFrame, ip int, objVal *Value, propName string, dest *Value) (bool, InterpretResult, Value) {
	// If frame is nil (called from outside VM loop), use current frame
	if frame == nil && vm.frameCount > 0 {
		frame = &vm.frames[vm.frameCount-1]
	}
	// Per-site inline cache stored on the current chunk (avoids global map lookup).
	siteIP := ip - 5 // OpGetProp is 1 (opcode) + 4 operands, ip is advanced past operands.
	cache := vm.getOrCreatePropInlineCache(frame, siteIP)

	if debugVM {
		fmt.Printf("[opGetProp] ip=%d obj=%s(%s) prop=%q\n", ip, objVal.Inspect(), objVal.TypeName(), propName)
	}

	// 0. GlobalThis special case: transparently access globals from heap
	// This makes globalThis.propertyName work for top-level var/function declarations
	if objVal.Type() == TypeObject {
		po := AsPlainObject(*objVal)
		if po == vm.GlobalObject {
			if debugVM {
				fmt.Printf("[DEBUG opGetProp globalThis] Looking for property '%s' in heap\n", propName)
				fmt.Printf("[DEBUG opGetProp globalThis] nameToIndex has %d entries\n", len(vm.heap.nameToIndex))
			}
			// Check if this property exists as a global in the heap
			if globalIdx, exists := vm.heap.nameToIndex[propName]; exists {
				if debugVM {
					fmt.Printf("[DEBUG opGetProp globalThis] Found '%s' at heap index %d\n", propName, globalIdx)
				}
				if value, ok := vm.heap.Get(globalIdx); ok {
					*dest = value
					return true, InterpretOK, *dest
				}
			} else if debugVM {
				fmt.Printf("[DEBUG opGetProp globalThis] Property '%s' NOT found in heap\n", propName)
			}
			// If not in heap, check Object.prototype for standard methods
			// GlobalObject has Null prototype to avoid issues, but should inherit Object.prototype methods
			if objProto := vm.ObjectPrototype.AsPlainObject(); objProto != nil {
				if value, exists := objProto.GetOwn(propName); exists {
					*dest = value
					return true, InterpretOK, *dest
				}
			}
			// If not in Object.prototype either, fall through to normal property access
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

	// 3. NativeFunctionWithProps (like String.fromCharCode, Function.prototype)
	if objVal.Type() == TypeNativeFunctionWithProps {
		nativeFnWithProps := objVal.AsNativeFunctionWithProps()

		// First check own properties
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

		// Walk prototype chain for inherited properties (like isPrototypeOf from Object.prototype)
		proto := nativeFnWithProps.Properties.GetPrototype()
		for proto.Type() != TypeNull && proto.Type() != TypeUndefined {
			if proto.Type() == TypeObject {
				if prop, exists := proto.AsPlainObject().GetOwn(propName); exists {
					*dest = prop
					return true, InterpretOK, *dest
				}
				proto = proto.AsPlainObject().GetPrototype()
			} else {
				break
			}
		}
	}

	// 4. Functions, Closures, Native Functions, Native Functions with Props, Async Native Functions, and Bound Functions (unified handling)
	if objVal.Type() == TypeFunction || objVal.Type() == TypeClosure || objVal.Type() == TypeBoundFunction || objVal.Type() == TypeNativeFunction || objVal.Type() == TypeNativeFunctionWithProps || objVal.Type() == TypeAsyncNativeFunction {
		// Set frame.ip before calling handleCallableProperty in case it throws (for exception handler lookup)
		if frame != nil {
			frame.ip = ip - 4
		}
		// Track helper call depth so exception handlers can set handlerFound
		vm.EnterHelperCall()
		result, handled := vm.handleCallableProperty(*objVal, propName)
		vm.ExitHelperCall()
		if handled {
			if debugVM {
				if propName == "name" || propName == "constructor" {
					if debugVM {
						fmt.Printf("[DBG opGetProp] '%s' via handleCallableProperty -> %s (%s)\n", propName, result.Inspect(), result.TypeName())
					}
				}
			}
			*dest = result
			return true, InterpretOK, *dest
		} else if vm.unwinding || vm.handlerFound {
			// Exception was thrown (e.g., strict mode caller/arguments access)
			if vm.handlerFound {
				vm.handlerFound = false
			}
			if !vm.unwinding {
				return false, InterpretOK, Undefined
			}
			return false, InterpretRuntimeError, Undefined
		}
	}

	// 5. Arguments object property lookup
	if objVal.Type() == TypeArguments {
		argObj := AsArguments(*objVal)
		// Handle special arguments object properties
		if propName == "length" {
			*dest = Number(float64(argObj.Length()))
			return true, InterpretOK, *dest
		}
		// Delegate to Object.prototype for inherited methods
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
			if frame != nil {
				frame.ip = ip - 4
			}
			vm.throwException(excVal)
			if !vm.unwinding {
				return false, InterpretOK, Undefined
			}
			return false, InterpretRuntimeError, Undefined
		case TypeString, TypeFloatNumber, TypeIntegerNumber, TypeBoolean, TypeSymbol, TypeBigInt:
			// For primitive types (string, number, boolean, symbol, bigint), accessing
			// unknown properties should return undefined (not throw an error).
			// Prototype methods and special properties were already handled above.
			if debugVM {
				fmt.Printf("[DBG opGetProp] Unknown property '%s' on primitive %s -> undefined\n", propName, objVal.TypeName())
			}
			*dest = Undefined
			return true, InterpretOK, *dest
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
			if frame != nil {
				frame.ip = ip - 4
			}
			vm.throwException(excVal)
			if !vm.unwinding {
				return false, InterpretOK, Undefined
			}
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
		if entry, hit := cache.lookupEntry(po.shape, propName); hit {
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
								if frame != nil {
									frame.ip = ip - 4
								}
								vm.throwException(ee.GetExceptionValue())
								if !vm.unwinding {
									return false, InterpretOK, Undefined
								}
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
							if frame != nil {
								frame.ip = ip - 4
							}
							vm.throwException(excVal)
							if !vm.unwinding {
								return false, InterpretOK, Undefined
							}
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
									if frame != nil {
										frame.ip = ip - 4
									}
									vm.throwException(ee.GetExceptionValue())
									if !vm.unwinding {
										return false, InterpretOK, Undefined
									}
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
								if frame != nil {
									frame.ip = ip - 4
								}
								vm.throwException(excVal)
								if !vm.unwinding {
									return false, InterpretOK, Undefined
								}
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
		// cacheKey uses siteIP to identify the site for prototype cache; 0 disables it when unknown.
		cacheKey := siteIP
		if cacheKey < 0 {
			cacheKey = 0
		}
		if holder, offset, isAccessor, found := vm.resolvePropertyMeta(*objVal, propName, cache, cacheKey); found {
			if isAccessor {
				if g, _, _, _, ok := holder.GetOwnAccessor(propName); ok && g.Type() != TypeUndefined {
					res, err := vm.Call(g, *objVal, nil)
					if err != nil {
						if ee, ok := err.(ExceptionError); ok {
							if frame != nil {
								frame.ip = ip - 4
							}
							vm.throwException(ee.GetExceptionValue())
							if !vm.unwinding {
								return false, InterpretOK, Undefined
							}
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
						if frame != nil {
							frame.ip = ip - 4
						}
						vm.throwException(excVal)
						if !vm.unwinding {
							return false, InterpretOK, Undefined
						}
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
						cache.updateCache(po.shape, propName, field.offset, field.isAccessor, field.writable)
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
		arr := objVal.AsArray()
		// Check if propName is a valid array index (numeric string like "0", "1", etc.)
		// Per ECMAScript, arr["0"] should work the same as arr[0]
		if len(propName) > 0 {
			isNumeric := true
			for _, c := range propName {
				if c < '0' || c > '9' {
					isNumeric = false
					break
				}
			}
			// Also reject leading zeros (except "0" itself) to match canonical array index
			if isNumeric && len(propName) > 1 && propName[0] == '0' {
				isNumeric = false
			}
			if isNumeric {
				idx := 0
				for _, c := range propName {
					idx = idx*10 + int(c-'0')
				}
				if idx < arr.Length() {
					*dest = arr.Get(idx)
					return true, InterpretOK, *dest
				}
			}
		}
		// Check for named properties (e.g., "index", "input" on match results)
		if v, ok := arr.GetOwn(propName); ok {
			*dest = v
			return true, InterpretOK, *dest
		}
		*dest = Undefined
		return true, InterpretOK, *dest
	}

	// 9. Map objects - consult Map.prototype chain for properties like forEach, get, set, etc.
	if objVal.Type() == TypeMap {
		proto := vm.MapPrototype
		if proto.IsObject() {
			po := proto.AsPlainObject()
			if v, ok := po.GetOwn(propName); ok {
				*dest = v
				return true, InterpretOK, *dest
			}
			// Walk the prototype chain
			current := po.prototype
			for current.typ != TypeNull && current.typ != TypeUndefined {
				if current.IsObject() {
					cpo := current.AsPlainObject()
					if v, ok := cpo.GetOwn(propName); ok {
						*dest = v
						return true, InterpretOK, *dest
					}
					current = cpo.prototype
				} else {
					break
				}
			}
		}
		*dest = Undefined
		return true, InterpretOK, *dest
	}

	// 10. Set objects - consult Set.prototype chain for properties like forEach, add, has, etc.
	if objVal.Type() == TypeSet {
		proto := vm.SetPrototype
		if proto.IsObject() {
			po := proto.AsPlainObject()
			if v, ok := po.GetOwn(propName); ok {
				*dest = v
				return true, InterpretOK, *dest
			}
			// Walk the prototype chain
			current := po.prototype
			for current.typ != TypeNull && current.typ != TypeUndefined {
				if current.IsObject() {
					cpo := current.AsPlainObject()
					if v, ok := cpo.GetOwn(propName); ok {
						*dest = v
						return true, InterpretOK, *dest
					}
					current = cpo.prototype
				} else {
					break
				}
			}
		}
		*dest = Undefined
		return true, InterpretOK, *dest
	}

	// 11. Generator objects
	if objVal.Type() == TypeGenerator {
		// Generator objects: consult Generator.prototype chain for regular properties
		proto := vm.GeneratorPrototype
		if proto.IsObject() {
			po := proto.AsPlainObject()
			if v, ok := po.GetOwn(propName); ok {
				*dest = v
				return true, InterpretOK, *dest
			}
			// Walk the prototype chain
			current := po.prototype
			for current.typ != TypeNull && current.typ != TypeUndefined {
				if current.IsObject() {
					if proto2 := current.AsPlainObject(); proto2 != nil {
						if v, ok := proto2.GetOwn(propName); ok {
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

	if objVal.Type() == TypeAsyncGenerator {
		// AsyncGenerator objects: consult AsyncGenerator.prototype chain for regular properties
		proto := vm.AsyncGeneratorPrototype
		if proto.IsObject() {
			po := proto.AsPlainObject()
			if v, ok := po.GetOwn(propName); ok {
				*dest = v
				return true, InterpretOK, *dest
			}
			// Walk the prototype chain
			current := po.prototype
			for current.typ != TypeNull && current.typ != TypeUndefined {
				if current.IsObject() {
					if proto2 := current.AsPlainObject(); proto2 != nil {
						if v, ok := proto2.GetOwn(propName); ok {
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

	// 12. RegExp objects (after special properties are handled)
	if objVal.Type() == TypeRegExp {
		regex := objVal.AsRegExpObject()
		if regex != nil && regex.Properties != nil {
			// Check for user-defined properties on the regexp
			if v, ok := regex.Properties.GetOwn(propName); ok {
				*dest = v
				return true, InterpretOK, *dest
			}
		}
		// Check RegExp.prototype for inherited methods
		if vm.RegExpPrototype.Type() == TypeObject {
			proto := vm.RegExpPrototype.AsPlainObject()
			if v, ok := proto.GetOwn(propName); ok {
				*dest = v
				return true, InterpretOK, *dest
			}
			// Walk prototype chain to Object.prototype
			current := proto.GetPrototype()
			for current.typ != TypeNull && current.typ != TypeUndefined {
				if current.IsObject() {
					if p := current.AsPlainObject(); p != nil {
						if v, ok := p.GetOwn(propName); ok {
							*dest = v
							return true, InterpretOK, *dest
						}
						current = p.GetPrototype()
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

	// 13. Proxy objects - delegate to handler
	if objVal.Type() == TypeProxy {
		proxy := objVal.AsProxy()
		if proxy.Revoked {
			// Proxy is revoked, throw TypeError
			var excVal Value
			if typeErrCtor, ok := vm.GetGlobal("TypeError"); ok {
				if res, callErr := vm.Call(typeErrCtor, Undefined, []Value{NewString("Cannot perform property access on a revoked Proxy")}); callErr == nil {
					excVal = res
				}
			}
			if excVal.Type() == 0 {
				eo := NewObject(vm.ErrorPrototype).AsPlainObject()
				eo.SetOwn("name", NewString("TypeError"))
				eo.SetOwn("message", NewString("Cannot perform property access on a revoked Proxy"))
				excVal = NewValueFromPlainObject(eo)
			}
			if frame != nil {
				frame.ip = ip - 4
			}
			vm.throwException(excVal)
			if !vm.unwinding {
				return false, InterpretOK, Undefined
			}
			return false, InterpretRuntimeError, Undefined
		}

		// Check if handler has a get trap
		getTrap, ok := proxy.handler.AsPlainObject().GetOwn("get")
		if ok {
			// Validate trap is callable
			if !getTrap.IsCallable() {
				var excVal Value
				if typeErrCtor, ok := vm.GetGlobal("TypeError"); ok {
					if res, callErr := vm.Call(typeErrCtor, Undefined, []Value{NewString("'get' on proxy: trap is not a function")}); callErr == nil {
						excVal = res
					}
				}
				if excVal.Type() == 0 {
					eo := NewObject(vm.ErrorPrototype).AsPlainObject()
					eo.SetOwn("name", NewString("TypeError"))
					eo.SetOwn("message", NewString("'get' on proxy: trap is not a function"))
					excVal = NewValueFromPlainObject(eo)
				}
				if frame != nil {
					frame.ip = ip - 4
				}
				vm.throwException(excVal)
				if !vm.unwinding {
					return false, InterpretOK, Undefined
				}
				return false, InterpretRuntimeError, Undefined
			}

			// Call the get trap: handler.get(target, propertyKey, receiver)
			trapArgs := []Value{proxy.target, NewString(propName), *objVal}
			result, err := vm.Call(getTrap, proxy.handler, trapArgs)
			if err != nil {
				if ee, ok := err.(ExceptionError); ok {
					if frame != nil {
						frame.ip = ip - 4
					}
					vm.throwException(ee.GetExceptionValue())
					if !vm.unwinding {
						return false, InterpretOK, Undefined
					}
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
				if frame != nil {
					frame.ip = ip - 4
				}
				vm.throwException(excVal)
				if !vm.unwinding {
					return false, InterpretOK, Undefined
				}
				return false, InterpretRuntimeError, Undefined
			}
			*dest = result
			return true, InterpretOK, *dest
		} else {
			// No get trap, fallback to target - implement directly to avoid recursion
			target := proxy.target
			if target.Type() == TypeObject {
				if result, handled := vm.handleSpecialProperties(target, propName); handled {
					*dest = result
					return true, InterpretOK, *dest
				}
				if result, handled := vm.handlePrimitiveMethod(target, propName); handled {
					*dest = result
					return true, InterpretOK, *dest
				}
				// Use enhanced property resolution with prototype caching and metadata
				if holder, offset, isAccessor, found := vm.resolvePropertyMeta(target, propName, nil, 0); found {
					if isAccessor {
						if g, _, _, _, ok := holder.GetOwnAccessor(propName); ok && g.Type() != TypeUndefined {
							res, err := vm.Call(g, target, nil)
							if err != nil {
								if ee, ok := err.(ExceptionError); ok {
									if frame != nil {
										frame.ip = ip - 4
									}
									vm.throwException(ee.GetExceptionValue())
									if !vm.unwinding {
										return false, InterpretOK, Undefined
									}
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
								if frame != nil {
									frame.ip = ip - 4
								}
								vm.throwException(excVal)
								if !vm.unwinding {
									return false, InterpretOK, Undefined
								}
								return false, InterpretRuntimeError, Undefined
							}
							*dest = res
						} else {
							*dest = Undefined
						}
					} else {
						*dest = holder.properties[offset]
					}
					return true, InterpretOK, *dest
				}
			} else if target.Type() == TypeDictObject {
				dict := target.AsDictObject()
				if fv, ok := dict.Get(propName); ok {
					*dest = fv
				} else {
					*dest = Undefined
				}
				return true, InterpretOK, *dest
			} else {
				*dest = Undefined
				return true, InterpretOK, *dest
			}
		}
	}

	// Shouldn't reach here, but handle as undefined
	*dest = Undefined
	return true, InterpretOK, *dest
}

// opGetPropSymbol handles property get where the key is a symbol Value.
func (vm *VM) opGetPropSymbol(frame *CallFrame, ip int, objVal *Value, symKey Value, dest *Value) (bool, InterpretResult, Value) {
	// If frame is nil (called from outside VM loop), use current frame
	if frame == nil && vm.frameCount > 0 {
		frame = &vm.frames[vm.frameCount-1]
	}
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
			if debugVM {
				fmt.Printf("[DBG opGetPropSymbol] Looking up Array.prototype=%p for symbol %s\n", po, symKey.AsSymbol())
			}
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
	case TypeTypedArray:
		// TypedArrays: consult appropriate TypedArray.prototype chain for symbol properties
		var proto Value
		ta := base.AsTypedArray()
		switch ta.GetElementType() {
		case TypedArrayFloat32:
			proto = vm.Float32ArrayPrototype
		// Add other typed array types as needed
		default:
			proto = vm.Float32ArrayPrototype // fallback
		}
		if proto.IsObject() {
			po := proto.AsPlainObject()
			if v, ok := po.GetOwnByKey(NewSymbolKey(symKey)); ok {
				*dest = v
				if debugVM {
					fmt.Printf("[DBG opGetPropSymbol] TypedArray.prototype[%s] -> %s (%s)\n", symKey.AsSymbol(), v.Inspect(), v.TypeName())
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
								fmt.Printf("[DBG opGetPropSymbol] TypedArray proto-chain[%s] -> %s (%s)\n", symKey.AsSymbol(), v.Inspect(), v.TypeName())
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
	case TypeAsyncGenerator:
		// Async generators: consult AsyncGenerator.prototype chain for symbol properties
		proto := vm.AsyncGeneratorPrototype
		if proto.IsObject() {
			po := proto.AsPlainObject()
			if v, ok := po.GetOwnByKey(NewSymbolKey(symKey)); ok {
				*dest = v
				if debugVM {
					fmt.Printf("[DBG opGetPropSymbol] AsyncGenerator.prototype[%s] -> %s (%s)\n", symKey.AsSymbol(), v.Inspect(), v.TypeName())
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
								fmt.Printf("[DBG opGetPropSymbol] AsyncGenerator proto-chain[%s] -> %s (%s)\n", symKey.AsSymbol(), v.Inspect(), v.TypeName())
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

	// RegExp: check own properties first, then RegExp.prototype chain for symbol properties
	if base.Type() == TypeRegExp {
		regex := base.AsRegExpObject()
		key := NewSymbolKey(symKey)
		// Check own properties first
		if regex != nil && regex.Properties != nil {
			if v, ok := regex.Properties.GetOwnByKey(key); ok {
				*dest = v
				return true, InterpretOK, *dest
			}
		}
		// Then check RegExp.prototype chain
		proto := vm.RegExpPrototype
		if proto.IsObject() {
			po := proto.AsPlainObject()
			if v, ok := po.GetOwnByKey(key); ok {
				*dest = v
				return true, InterpretOK, *dest
			}
			current := po.prototype
			for current.typ != TypeNull && current.typ != TypeUndefined {
				if current.IsObject() {
					if proto2 := current.AsPlainObject(); proto2 != nil {
						if v, ok := proto2.GetOwnByKey(key); ok {
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
						if frame != nil {
							frame.ip = ip - 4
						}
						vm.throwException(ee.GetExceptionValue())
						if !vm.unwinding {
							return false, InterpretOK, Undefined
						}
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
					if frame != nil {
						frame.ip = ip - 4
					}
					vm.throwException(excVal)
					if !vm.unwinding {
						return false, InterpretOK, Undefined
					}
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
								if frame != nil {
									frame.ip = ip - 4
								}
								vm.throwException(ee.GetExceptionValue())
								if !vm.unwinding {
									return false, InterpretOK, Undefined
								}
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
							if frame != nil {
								frame.ip = ip - 4
							}
							vm.throwException(excVal)
							if !vm.unwinding {
								return false, InterpretOK, Undefined
							}
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

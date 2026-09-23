package vm

import "fmt"

func (vm *VM) opGetProp(frame *CallFrame, ip int, objVal *Value, propName string, dest *Value) (bool, InterpretResult, Value) {
	// Track if frame was originally nil - if so, we should NOT update frame.ip
	// because we're being called from a helper function (like toPrimitive) not the VM loop
	frameWasNil := frame == nil
	// If frame is nil (called from outside VM loop), use current frame for cache lookup only
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
			if vm.ObjectPrototype.Type() == TypeObject {
				objProto := vm.ObjectPrototype.AsPlainObject()
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

		// DEBUG removed

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

	// 3b. NativeFunction with lazily-created Properties (user-set properties like nf.x = 1)
	if objVal.Type() == TypeNativeFunction {
		nf := objVal.AsNativeFunction()
		if nf != nil && nf.Properties != nil {
			// Check for an accessor property first (getters/setters) -
			// mirrors TypeBoundFunction's equivalent check in
			// handleCallableProperty (property_helpers.go). Without this,
			// an accessor defined via Object.defineProperty(nf, "custom",
			// {get(){...}}) fell straight to GetOwn below, which - for an
			// accessor field - returns (Undefined, true) (DefineAccessorProperty
			// appends a placeholder Undefined properties slot for it), so
			// `nf.custom` silently answered undefined instead of calling
			// the getter, even though the same accessor's *setter* already
			// ran correctly via `nf.custom = v` (pkg/vm/op_setprop.go).
			if g, _, _, _, ok := nf.Properties.GetOwnAccessor(propName); ok {
				if g.Type() != TypeUndefined {
					res, err := vm.Call(g, *objVal, nil)
					if err != nil {
						if ee, ok := err.(ExceptionError); ok {
							if frame != nil && !frameWasNil {
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
						if frame != nil && !frameWasNil {
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
				// Setter-only accessor (no getter): reads as undefined per spec.
				*dest = Undefined
				return true, InterpretOK, *dest
			}
			if prop, exists := nf.Properties.GetOwn(propName); exists {
				*dest = prop
				return true, InterpretOK, *dest
			}
		}
	}

	// 4. Functions, Closures, Native Functions, Native Functions with Props, Async Native Functions, and Bound Functions (unified handling)
	if objVal.Type() == TypeFunction || objVal.Type() == TypeClosure || objVal.Type() == TypeBoundFunction || objVal.Type() == TypeNativeFunction || objVal.Type() == TypeNativeFunctionWithProps || objVal.Type() == TypeAsyncNativeFunction {
		// Set frame.ip before calling handleCallableProperty in case it throws (for exception handler lookup)
		// Only update if frame was NOT nil (i.e., we're in the VM loop, not a helper function)
		if frame != nil && !frameWasNil {
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

	// 5. Arguments object: own property per ArgumentsOwnProperty (indices,
	// length, callee, named, redefined or deleted), else its real
	// [[Prototype]] chain (paserati#535).
	if objVal.Type() == TypeArguments {
		return vm.argumentsGetProp(frame, ip, frameWasNil, *objVal, propName, dest)
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
			if frame != nil && !frameWasNil {
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
			if frame != nil && !frameWasNil {
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
								if frame != nil && !frameWasNil {
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
							if frame != nil && !frameWasNil {
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
					if pv.Type() != TypeObject {
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
									if frame != nil && !frameWasNil {
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
								if frame != nil && !frameWasNil {
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
							if frame != nil && !frameWasNil {
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
						if frame != nil && !frameWasNil {
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

		// resolvePropertyMeta didn't find property - fall back to a full generic
		// chain walk, which handles prototypes of any object kind (functions,
		// arrays, etc. can all be used as prototypes in JavaScript)
		if fv, ok := vm.getInheritedGeneric(NewValueFromPlainObject(po), propName); ok {
			*dest = fv
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
		// An index (or named key) explicitly turned into an accessor via
		// Object.defineProperty (see ArrayDefineOwnProperty) takes priority
		// over a plain element/named-property read below. Guarded behind
		// HasAccessors() so the overwhelmingly common array - which never
		// had one defined - pays only a nil check here, not a map probe.
		if arr.HasAccessors() {
			if g, _, _, _, ok := arr.GetOwnAccessor(propName); ok {
				if g.Type() == TypeUndefined {
					*dest = Undefined
					return true, InterpretOK, *dest
				}
				if frame != nil && !frameWasNil {
					frame.ip = ip - 4
				}
				result, err := vm.Call(g, *objVal, nil)
				if err != nil {
					if ee, ok := err.(ExceptionError); ok {
						vm.throwException(ee.GetExceptionValue())
						if !vm.unwinding {
							// Exception was caught by a handler; caller
							// (goto reloadFrame on false+InterpretOK) reloads.
							return false, InterpretOK, Undefined
						}
						return false, InterpretRuntimeError, Undefined
					}
					status := vm.runtimeError("%v", err)
					return false, status, Undefined
				}
				*dest = result
				return true, InterpretOK, *dest
			}
		}
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
					if idx < len(arr.elements) && arr.elements[idx].typ != TypeHole {
						*dest = arr.elements[idx]
						return true, InterpretOK, *dest
					}
					// idx is within .length but not backed by a dense
					// element - either a genuine sparse hole, or (see
					// paserati#176) an index that DefineOwnProperty stored
					// as a named property instead of growing .elements
					// (ArrayObject.Set is O(idx); see
					// maxDenseArrayDefineIndex/maxDenseArraySetIndex).
					// Consult the named-property store before defaulting
					// to Undefined - falling through to the same
					// arr.GetOwn(propName) check just below would also
					// work, but returning here keeps this branch
					// self-contained and mirrors OpGetIndex's identical fix.
					if v, ok := arr.GetOwn(propName); ok {
						*dest = v
						return true, InterpretOK, *dest
					}
					*dest = Undefined
					return true, InterpretOK, *dest
				}
			}
		}
		// Check for named properties (e.g., "index", "input" on match results)
		if v, ok := arr.GetOwn(propName); ok {
			*dest = v
			return true, InterpretOK, *dest
		}
		// Walk the prototype chain via the shared, non-panicking helper -
		// not a hand-rolled walk assuming arr.prototype (when set, e.g. by
		// `class S extends Array {}`, or explicitly via
		// Object.setPrototypeOf) is always a PlainObject or unset. Since
		// Object.setPrototypeOf can now store *any* object-kind value or an
		// explicit null there (#418), the previous `!proto.IsObject()`
		// fallback-to-intrinsic check also mishandled an explicit null
		// override (indistinguishable from "unset") and the walk itself
		// would panic in AsPlainObject() the first time it reached a
		// non-PlainObject link (e.g. another Array or a Map as the
		// prototype). finishProtoChainGet -> plainPrototypeOf goes through
		// InstancePrototypeOverride, which now tells null and unset apart,
		// and stops cleanly instead of panicking on any other kind.
		return vm.finishProtoChainGet(frame, ip, frameWasNil, propName, *objVal, dest)
	}

	// 9-11. Map, Set, Promise, WeakMap, WeakSet, WeakRef,
	// FinalizationRegistry, SharedArrayBuffer, ArrayBuffer, DataView and
	// (async) generators: ordinary objects apart from their internal slots.
	// An own property lives on the side table (OwnPropertiesTable) - an own
	// accessor's getter runs with the instance as `this` - and otherwise the
	// instance's own [[Prototype]] chain is walked via finishProtoChainGet
	// (see the Array case above for why a hand-rolled walk is unsafe since
	// #418). WeakMap/WeakSet/WeakRef/FinalizationRegistry/DataView and the
	// generators used to skip the own lookup entirely, so an expando like
	// `dv.x = 1` read back as undefined (paserati#529).
	switch objVal.Type() {
	case TypeMap, TypeSet, TypePromise, TypeWeakMap, TypeWeakSet, TypeWeakRef, TypeFinalizationRegistry,
		TypeSharedArrayBuffer, TypeArrayBuffer, TypeDataView, TypeGenerator, TypeAsyncGenerator:
		if props := OwnPropertiesTable(*objVal); props != nil {
			if handled, ok, status, val := vm.getSideTableOwn(frame, ip, frameWasNil, props, keyFromString(propName), *objVal, dest); handled {
				return ok, status, val
			}
		}
		return vm.finishProtoChainGet(frame, ip, frameWasNil, propName, *objVal, dest)
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
		// The instance's own [[Prototype]] rather than the hardcoded
		// RegExp.prototype, and a walk that invokes accessors - see
		// finishProtoChainGet. This is what makes `class S extends RegExp {}`
		// reach S.prototype's methods, and (per spec) lets
		// String.prototype.replace/match/split dispatch through a subclass's
		// own `exec` override.
		return vm.finishProtoChainGet(frame, ip, frameWasNil, propName, *objVal, dest)
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
			if frame != nil && !frameWasNil {
				frame.ip = ip - 4
			}
			vm.throwException(excVal)
			if !vm.unwinding {
				return false, InterpretOK, Undefined
			}
			return false, InterpretRuntimeError, Undefined
		}

		// Check if handler has a get trap. GetMethod(handler, "get") per
		// spec: an inherited trap counts, not just an own one - proxyGetTrap
		// (not a bare proxy.handler.AsPlainObject().GetOwn("get")) for the
		// same reason documented on its own definition.
		getTrap, hasGetTrap := proxyGetTrap(proxy.handler, "get")
		if hasGetTrap && getTrap.Type() != TypeUndefined && getTrap.Type() != TypeNull {
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
				if frame != nil && !frameWasNil {
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
					if frame != nil && !frameWasNil {
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
				if frame != nil && !frameWasNil {
					frame.ip = ip - 4
				}
				vm.throwException(excVal)
				if !vm.unwinding {
					return false, InterpretOK, Undefined
				}
				return false, InterpretRuntimeError, Undefined
			}
			// ECMAScript 10.5.8 invariant validation
			if proxy.target.Type() == TypeObject {
				targetObj := proxy.target.AsPlainObject()
				if g, _, _, c, isAccessor := targetObj.GetOwnAccessor(propName); isAccessor && !c {
					if g.Type() == TypeUndefined && !result.IsUndefined() {
						if frame != nil && !frameWasNil {
							frame.ip = ip - 4
						}
						vm.ThrowTypeError("'get' on proxy: property '" + propName + "' is a non-configurable accessor without a getter, but the trap returned a non-undefined value")
						if !vm.unwinding {
							return false, InterpretOK, Undefined
						}
						return false, InterpretRuntimeError, Undefined
					}
				} else if v, w, _, c, found := targetObj.GetOwnDescriptor(propName); found && !c && !w {
					if !v.StrictlyEquals(result) {
						if frame != nil && !frameWasNil {
							frame.ip = ip - 4
						}
						vm.ThrowTypeError("'get' on proxy: property '" + propName + "' is a read-only and non-configurable data property on the proxy target but the proxy did not return its actual value")
						if !vm.unwinding {
							return false, InterpretOK, Undefined
						}
						return false, InterpretRuntimeError, Undefined
					}
				}
			}
			*dest = result
			return true, InterpretOK, *dest
		} else {
			// No get trap: fall back to target.[[Get]] via
			// getPropertyWithReceiver, not a hand-rolled reimplementation
			// (what used to live here, added to "avoid recursion" - but
			// it only ever handled a TypeObject/TypeDictObject/callable
			// target; every other legal proxy.target kind (TypeArray,
			// TypeMap, TypeSet, TypePromise, TypeRegExp, TypeGenerator,
			// TypeBoundFunction, TypeNativeFunction,
			// TypeNativeFunctionWithProps, TypeArguments, a further-nested
			// Proxy, ...) fell through to a bare `*dest = Undefined`
			// instead - e.g. `new Proxy(someArray, {})` (no get trap, a
			// real Array target) silently read `undefined` for EVERY
			// property, including "length": handleSpecialProperties and
			// handlePrimitiveMethod, both called here, switch on the
			// TARGET's kind (TypeArray/TypeMap/TypeSet/... for the
			// former, TypeString/TypeArray/TypeMap/... for the latter)
			// but were only ever reached when target.Type() == TypeObject
			// was already true - so neither call could ever actually
			// match anything; both were dead code at this specific call
			// site. getPropertyWithReceiver already handles every kind
			// (including recursing through a further-nested Proxy) and
			// already threads a receiver through any accessor/trap found
			// along the way, exactly like the get-trap-call branch above
			// does with *objVal - so this isn't a behavior change for the
			// TypeObject/TypeDictObject/callable kinds that WERE already
			// handled here, only an extension to the kinds that weren't.
			result, err := vm.getPropertyWithReceiver(proxy.target, propName, *objVal)
			if err != nil {
				if ee, ok := err.(ExceptionError); ok {
					if frame != nil && !frameWasNil {
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
				if frame != nil && !frameWasNil {
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
		}
	}

	// Shouldn't reach here, but handle as undefined
	*dest = Undefined
	return true, InterpretOK, *dest
}

// opGetPropSymbol handles property get where the key is a symbol Value.
func (vm *VM) opGetPropSymbol(frame *CallFrame, ip int, objVal *Value, symKey Value, dest *Value) (bool, InterpretResult, Value) {
	// Track if frame was originally nil - if so, we should NOT update frame.ip
	// because we're being called from a helper function (like toPrimitive) not the VM loop
	frameWasNil := frame == nil
	// If frame is nil (called from outside VM loop), use current frame for cache lookup only
	if frame == nil && vm.frameCount > 0 {
		frame = &vm.frames[vm.frameCount-1]
	}
	// Prepare a per-site cache key for symbol lookups (future use)
	_ = generateSymbolCacheKey // reference to avoid unused warning if not used yet
	// cacheKey := generateSymbolCacheKey(ip, symKey)
	// Resolve a prototype-backed view for primitives
	base := *objVal
	switch base.Type() {
	case TypeMap, TypeSet, TypePromise, TypeWeakMap, TypeWeakSet, TypeWeakRef, TypeFinalizationRegistry,
		TypeSharedArrayBuffer, TypeArrayBuffer, TypeDataView, TypeTypedArray, TypeGenerator, TypeAsyncGenerator:
		// Side-table kinds (see OwnPropertiesTable): own symbol property
		// first, then the instance's actual [[Prototype]] chain, with any
		// getter found called on the instance. The per-kind walks further
		// down skipped the own table (so `m[sym] = v; m[sym]` read
		// undefined - paserati#528/#529), ignored subclass prototypes, and
		// returned an accessor's raw slot instead of calling its getter.
		key := NewSymbolKey(symKey)
		if props := OwnPropertiesTable(base); props != nil {
			if handled, ok, status, val := vm.getSideTableOwn(frame, ip, frameWasNil, props, key, base, dest); handled {
				return ok, status, val
			}
		}
		slot := vm.findSymbolSlot(vm.PrototypeOf(base), key)
		if !slot.found {
			*dest = Undefined
			return true, InterpretOK, *dest
		}
		if !slot.isAccessor {
			*dest = slot.value
			return true, InterpretOK, *dest
		}
		return vm.invokeSymbolGetter(frame, ip, frameWasNil, slot.getter, base, dest)
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
					if current.Type() == TypeObject {
						proto2 := current.AsPlainObject()
						if v, ok := proto2.GetOwnByKey(NewSymbolKey(symKey)); ok {
							*dest = v
							if debugVM {
								fmt.Printf("[DBG opGetPropSymbol] String proto-chain[%s] -> %s (%s)\n", symKey.AsSymbol(), v.Inspect(), v.TypeName())
							}
							return true, InterpretOK, *dest
						}
						current = proto2.prototype
					} else if current.Type() == TypeDictObject {
						dict := current.AsDictObject()
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
		// Arrays: consult the array's OWN symbol-keyed properties first -
		// opSetPropSymbol's TypeArray case (pkg/vm/op_setprop.go) already
		// writes `arr[sym] = v` into ArrayObject.symbolProps via
		// SetSymbolProp, but this case used to skip straight to the
		// prototype chain without ever checking it, so the read half of
		// the exact same feature silently returned undefined - or, worse,
		// a same-named symbol property inherited from Array.prototype -
		// instead of the array's own value:
		//
		//   const arr = [1, 2, 3];
		//   const sym = Symbol("s");
		//   arr[sym] = 42;
		//   arr[sym]; // before: undefined - Node: 42
		//
		// GetSymbolProp already existed and worked (that's how
		// HasOwnSymbolProp/Object.getOwnPropertySymbols's new TypeArray
		// case can see it) - this case just never called it.
		arrObj := base.AsArray()
		if sym := symKey.AsSymbolObject(); sym != nil {
			// A symbol-keyed accessor defined via Object.defineProperty(arr,
			// sym, {get, set}) takes priority over the plain symbolProps
			// value, mirroring the named-property read path (GetOwnAccessor
			// consulted before a plain element/property) - see
			// ArrayObject.GetOwnSymbolAccessor and
			// ArrayDefineOwnSymbolProperty (array_props.go).
			if arrObj.HasSymbolAccessors() {
				if getter, _, _, _, ok := arrObj.GetOwnSymbolAccessor(sym); ok {
					if getter.Type() == TypeUndefined {
						*dest = Undefined
						return true, InterpretOK, *dest
					}
					res, err := vm.Call(getter, base, nil)
					if err != nil {
						if ee, ok := err.(ExceptionError); ok {
							vm.throwException(ee.GetExceptionValue())
							return false, InterpretRuntimeError, Undefined
						}
						vm.ThrowTypeError(err.Error())
						return false, InterpretRuntimeError, Undefined
					}
					*dest = res
					return true, InterpretOK, *dest
				}
			}
			if v, ok := arrObj.GetSymbolProp(sym); ok {
				*dest = v
				return true, InterpretOK, *dest
			}
		}
		// Arrays: consult the per-instance prototype override (subclassing)
		// before falling back to the realm's intrinsic Array.prototype.
		proto := arrObj.prototype
		if !proto.IsObject() {
			proto = vm.ArrayPrototype
		}
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
					if current.Type() == TypeObject {
						proto2 := current.AsPlainObject()
						if v, ok := proto2.GetOwnByKey(NewSymbolKey(symKey)); ok {
							*dest = v
							if debugVM {
								fmt.Printf("[DBG opGetPropSymbol] Array proto-chain[%s] -> %s (%s)\n", symKey.AsSymbol(), v.Inspect(), v.TypeName())
							}
							return true, InterpretOK, *dest
						}
						current = proto2.prototype
					} else if current.Type() == TypeDictObject {
						dict := current.AsDictObject()
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
	case TypeArguments:
		// Own symbol property first (e.g. Symbol.iterator, set at creation),
		// then the object's real [[Prototype]] chain - Object.prototype, not
		// Array.prototype, which this used to walk (paserati#535).
		argObj := base.AsArguments()
		if v, ok := argObj.GetSymbolProp(symKey.AsSymbolObject()); ok {
			*dest = v
			return true, InterpretOK, *dest
		}
		slot := vm.findSymbolSlot(vm.PrototypeOf(base), NewSymbolKey(symKey))
		if !slot.found {
			*dest = Undefined
			return true, InterpretOK, *dest
		}
		if !slot.isAccessor {
			*dest = slot.value
			return true, InterpretOK, *dest
		}
		return vm.invokeSymbolGetter(frame, ip, frameWasNil, slot.getter, base, dest)
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
					if current.Type() == TypeObject {
						proto2 := current.AsPlainObject()
						if v, ok := proto2.GetOwnByKey(key); ok {
							*dest = v
							return true, InterpretOK, *dest
						}
						current = proto2.prototype
					} else if current.Type() == TypeDictObject {
						dict := current.AsDictObject()
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
						if frame != nil && !frameWasNil {
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
					if frame != nil && !frameWasNil {
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
			if current.Type() == TypeObject {
				proto := current.AsPlainObject()
				if g, _, _, _, ok := proto.GetOwnAccessorByKey(key); ok {
					if g.Type() != TypeUndefined {
						res, err := vm.Call(g, base, nil)
						if err != nil {
							if ee, ok := err.(ExceptionError); ok {
								if frame != nil && !frameWasNil {
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
							if frame != nil && !frameWasNil {
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
			if current.Type() == TypeDictObject {
				dict := current.AsDictObject()
				current = dict.prototype
				continue
			}
			break
		}
		*dest = Undefined
		return true, InterpretOK, *dest
	}

	// Callable objects (plain functions, closures, native functions, bound
	// functions and the property-carrying built-in constructors) all resolve a
	// symbol key the same way: own properties first, then the real
	// [[Prototype]] chain, invoking any accessor's getter with this = base.
	//
	// Before this was unified, each flavour had its own copy of the walk and
	// only TypeNativeFunction invoked a getter - and even that only for an OWN
	// accessor. Inherited accessors were unreachable because the shared
	// lookupSymbolOnProtoChain took no receiver, and TypeFunction/TypeClosure
	// hardcoded Function.prototype as the parent instead of the constructor's
	// actual [[Prototype]], so `class Foo extends Uint8Array {}` could never
	// see anything Uint8Array or %TypedArray% defined.
	switch base.Type() {
	case TypeFunction, TypeClosure, TypeNativeFunction, TypeNativeFunctionWithProps, TypeBoundFunction:
		return vm.resolveSymbolSlot(frame, ip, frameWasNil, base, NewSymbolKey(symKey), dest)
	}

	// DictObject: no symbol identity support yet
	*dest = Undefined
	return true, InterpretOK, *dest
}

// getSideTableOwn reads key from an exotic value's own-property side table:
// a data property's value, or an own accessor's getter called with receiver
// as `this`. handled is false when props has no such own property, in which
// case the caller continues up the [[Prototype]] chain.
func (vm *VM) getSideTableOwn(frame *CallFrame, ip int, frameWasNil bool, props *PlainObject, key PropertyKey, receiver Value, dest *Value) (handled bool, ok bool, status InterpretResult, val Value) {
	if getter, _, _, _, isAcc := props.GetOwnAccessorByKey(key); isAcc {
		ok, status, val = vm.invokeSymbolGetter(frame, ip, frameWasNil, getter, receiver, dest)
		return true, ok, status, val
	}
	if v, exists := props.GetOwnByKey(key); exists {
		*dest = v
		return true, true, InterpretOK, v
	}
	return false, false, InterpretOK, Undefined
}

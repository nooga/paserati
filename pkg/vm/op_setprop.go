package vm

import "fmt"

const debugOpSetProp = false

func (vm *VM) opSetProp(ip int, objVal *Value, propName string, valueToSet *Value) (bool, InterpretResult, Value) {
	if debugOpSetProp {
		fmt.Printf("[DEBUG opSetProp] ENTRY: propName=%q, objType=%s, valueType=%s\n", propName, objVal.TypeName(), valueToSet.TypeName())
	}

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

	// GlobalThis special case: transparently write to globals in heap
	// This makes globalThis.propertyName = value work for top-level var/function declarations
	if objVal.Type() == TypeObject {
		po := AsPlainObject(*objVal)
		if debugOpSetProp {
			fmt.Printf("[DEBUG opSetProp] GlobalObject check: po=%p, vm.GlobalObject=%p, match=%v\n", po, vm.GlobalObject, po == vm.GlobalObject)
		}
		if po == vm.GlobalObject {
			// Check if this property exists as a global in the heap
			if debugOpSetProp {
				_, exists := vm.heap.nameToIndex[propName]
				fmt.Printf("[DEBUG opSetProp] Heap check for %q: exists=%v\n", propName, exists)
			}
			if globalIdx, exists := vm.heap.nameToIndex[propName]; exists {
				// Check if the property is writable before updating
				// Global constants like undefined, NaN, Infinity are non-writable
				for _, f := range po.shape.fields {
					if f.keyKind == KeyKindString && f.name == propName {
						if !f.writable {
							// Property is non-writable - throw TypeError
							err := vm.NewTypeError(fmt.Sprintf("Cannot assign to read only property '%s'", propName))
							if excErr, ok := err.(ExceptionError); ok {
								vm.throwException(excErr.GetExceptionValue())
								return false, InterpretRuntimeError, Undefined
							}
						}
						break
					}
				}
				// Update existing global in heap AND the PlainObject
				// Both need to be in sync so reads via bracket notation work
				vm.heap.Set(globalIdx, *valueToSet)
				po.SetOwn(propName, *valueToSet)
				return true, InterpretOK, *valueToSet
			}
			// If not in heap, we could allocate a new global slot here
			// For now, fall through to normal property setting for new properties on globalThis
		}
	}

	// Per-site inline cache stored on the current chunk (avoids global map lookup).
	siteIP := ip - 5 // OpSetProp is 1 (opcode) + 4 operands, ip is advanced past operands.
	var frame *CallFrame
	if vm.frameCount > 0 {
		frame = &vm.frames[vm.frameCount-1]
	}
	cache := vm.getOrCreatePropInlineCache(frame, siteIP)

	// Handle property setting on function-like values
	switch objVal.Type() {
	case TypeFunction:
		fn := AsFunction(*objVal)
		if fn.Properties == nil {
			fn.Properties = NewObject(Undefined).AsPlainObject()
		}
		// Check for accessor property with a setter first (BEFORE strict mode caller/arguments check)
		// User-defined static setters named "arguments" or "caller" are allowed
		if _, setter, _, _, ok := fn.Properties.GetOwnAccessor(propName); ok && setter.Type() != TypeUndefined {
			_, err := vm.Call(setter, *objVal, []Value{*valueToSet})
			if err != nil {
				if ee, ok := err.(ExceptionError); ok {
					vm.throwException(ee.GetExceptionValue())
					return false, InterpretRuntimeError, Undefined
				}
				vm.throwException(NewString(err.Error()))
				return false, InterpretRuntimeError, Undefined
			}
			return true, InterpretOK, *valueToSet
		}
		// ES5 strict mode restriction: writing to "caller" or "arguments" on strict functions throws TypeError
		// This check is AFTER the accessor check so user-defined setters take precedence
		if (propName == "caller" || propName == "arguments") && fn.Chunk != nil && fn.Chunk.IsStrict {
			vm.ThrowTypeError("'caller', 'callee', and 'arguments' properties may not be accessed on strict mode functions or the arguments objects for calls to them")
			return false, InterpretRuntimeError, Undefined
		}
		if propName == "prototype" {
			// For class constructors, prototype must be: writable=false, enumerable=false, configurable=false
			w, e, c := false, false, false
			fn.Properties.DefineOwnProperty("prototype", *valueToSet, &w, &e, &c)
		} else {
			fn.Properties.SetOwn(propName, *valueToSet)
		}
		return true, InterpretOK, *valueToSet
	case TypeClosure:
		closure := AsClosure(*objVal)
		// Use closure's own Properties to avoid sharing with other closures using same FunctionObject
		if closure.Properties == nil {
			closure.Properties = NewObject(Undefined).AsPlainObject()
		}
		// Check for accessor property with a setter first (BEFORE strict mode caller/arguments check)
		// User-defined static setters named "arguments" or "caller" are allowed in classes
		if _, setter, _, _, ok := closure.Properties.GetOwnAccessor(propName); ok && setter.Type() != TypeUndefined {
			_, err := vm.Call(setter, *objVal, []Value{*valueToSet})
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
		// ES5 strict mode restriction: writing to "caller" or "arguments" on strict functions throws TypeError
		// This check is AFTER the accessor check so user-defined setters take precedence
		if (propName == "caller" || propName == "arguments") && closure.Fn.Chunk != nil && closure.Fn.Chunk.IsStrict {
			vm.ThrowTypeError("'caller', 'callee', and 'arguments' properties may not be accessed on strict mode functions or the arguments objects for calls to them")
			return false, InterpretRuntimeError, Undefined
		}
		if propName == "prototype" {
			// For class constructors, prototype must be: writable=false, enumerable=false, configurable=false
			w, e, c := false, false, false
			closure.Properties.DefineOwnProperty("prototype", *valueToSet, &w, &e, &c)
		} else {
			closure.Properties.SetOwn(propName, *valueToSet)
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
		if debugOpSetProp {
			fmt.Printf("[DEBUG opSetProp] Object type is TypeObject, entering PlainObject path\n")
		}
		po := AsPlainObject(*objVal)

		// Try cache lookup for existing property write (check accessor/writable flags)
		if entry, hit := cache.lookupEntry(po.shape, propName); hit {
			if debugOpSetProp {
				fmt.Printf("[DEBUG opSetProp] Cache HIT for prop=%q: shape=%p, offset=%d, writable=%v, isAccessor=%v\n",
					propName, po.shape, entry.offset, entry.writable, entry.isAccessor)
			}
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
					if debugOpSetProp {
						fmt.Printf("[DEBUG opSetProp] Cache hit fast path for property %q, writable=%v\n", propName, entry.writable)
					}
					po.properties[entry.offset] = *valueToSet
					return true, InterpretOK, *valueToSet
				}
			}
			// Cache was stale or property layout changed, fall through to slow path
			if debugOpSetProp {
				fmt.Printf("[DEBUG opSetProp] Cache hit but not taking fast path: isAccessor=%v, writable=%v\n", entry.isAccessor, entry.writable)
			}
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
				// No setter: throw TypeError in strict mode (we assume strict mode)
				if debugOpSetProp {
					fmt.Printf("[DEBUG opSetProp] Setter is undefined, throwing TypeError\n")
				}
				err := vm.NewTypeError(fmt.Sprintf("Cannot set property '%s' which has only a getter", propName))
				if excErr, ok := err.(ExceptionError); ok {
					vm.throwException(excErr.GetExceptionValue())
					return false, InterpretRuntimeError, Undefined
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

		// Check if property exists on object or prototype and is non-writable (strict mode - always throw)
		propertyExists := false
		for _, f := range po.shape.fields {
			if f.keyKind == KeyKindString && f.name == propName {
				propertyExists = true
				if !f.writable {
					// Property exists but is not writable - throw TypeError
					err := vm.NewTypeError(fmt.Sprintf("Cannot assign to read only property '%s'", propName))
					if excErr, ok := err.(ExceptionError); ok {
						vm.throwException(excErr.GetExceptionValue())
						return false, InterpretRuntimeError, Undefined
					}
				}
				break
			}
		}

		// If property doesn't exist on object, check prototype chain for non-writable data property
		// Per ECMAScript spec, you cannot shadow a non-writable property from prototype
		if !propertyExists {
			protoChain := po.GetPrototype()
			for protoChain.IsObject() {
				protoObj := protoChain.AsPlainObject()
				if protoObj == nil {
					break
				}
				// Check if property exists on this prototype
				for _, f := range protoObj.shape.fields {
					if f.keyKind == KeyKindString && f.name == propName {
						// Found property on prototype
						if !f.isAccessor && !f.writable {
							// Non-writable data property on prototype - cannot shadow it
							err := vm.NewTypeError(fmt.Sprintf("Cannot assign to read only property '%s'", propName))
							if excErr, ok := err.(ExceptionError); ok {
								vm.throwException(excErr.GetExceptionValue())
								return false, InterpretRuntimeError, Undefined
							}
						}
						break
					}
				}
				protoChain = protoObj.GetPrototype()
			}
		}

		// Check if we're trying to add a new property to a non-extensible object
		if !propertyExists && !po.IsExtensible() {
			err := vm.NewTypeError(fmt.Sprintf("Cannot add property '%s', object is not extensible", propName))
			if excErr, ok := err.(ExceptionError); ok {
				vm.throwException(excErr.GetExceptionValue())
				return false, InterpretRuntimeError, Undefined
			}
		}

		po.SetOwn(propName, *valueToSet)

		// Update cache if shape didn't change (existing property)
		// or if shape changed (new property added)
		for _, field := range po.shape.fields {
			if field.name == propName {
				cache.updateCache(po.shape, propName, field.offset, field.isAccessor, field.writable)
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
	if debugOpSetProp {
		fmt.Printf("[DEBUG opSetProp] Entering fallback path, objType=%d\n", objVal.Type())
	}
	switch objVal.Type() {
	case TypeArray:
		arr := objVal.AsArray()
		if propName == "length" {
			// Setting length truncates or expands the array
			// Frozen arrays can't have length changed
			if !arr.IsExtensible() {
				// In strict mode, throw TypeError; in non-strict, silently fail
				if vm.IsInStrictMode() {
					err := vm.NewTypeError(fmt.Sprintf("Cannot assign to read only property 'length'"))
					if excErr, ok := err.(ExceptionError); ok {
						vm.throwException(excErr.GetExceptionValue())
						return false, InterpretRuntimeError, Undefined
					}
				}
				return true, InterpretOK, *valueToSet // Silently fail in non-strict
			}
			newLen := int(valueToSet.ToFloat())
			if newLen < 0 {
				newLen = 0
			}
			arr.SetLength(newLen)
		} else {
			// Check if property exists
			_, exists := arr.GetOwn(propName)
			// Check if we're trying to add a new property to a non-extensible array
			if !exists && !arr.IsExtensible() {
				// In strict mode, throw TypeError; in non-strict, silently fail
				if vm.IsInStrictMode() {
					err := vm.NewTypeError(fmt.Sprintf("Cannot add property '%s', object is not extensible", propName))
					if excErr, ok := err.(ExceptionError); ok {
						vm.throwException(excErr.GetExceptionValue())
						return false, InterpretRuntimeError, Undefined
					}
				}
				return true, InterpretOK, *valueToSet // Silently fail in non-strict
			}
			// Frozen arrays also can't have existing properties changed
			if exists && !arr.IsExtensible() {
				// In strict mode, throw TypeError; in non-strict, silently fail
				if vm.IsInStrictMode() {
					err := vm.NewTypeError(fmt.Sprintf("Cannot assign to read only property '%s'", propName))
					if excErr, ok := err.(ExceptionError); ok {
						vm.throwException(excErr.GetExceptionValue())
						return false, InterpretRuntimeError, Undefined
					}
				}
				return true, InterpretOK, *valueToSet // Silently fail in non-strict
			}
			arr.SetOwn(propName, *valueToSet)
		}
		return true, InterpretOK, *valueToSet
	case TypeDictObject:
		if debugOpSetProp {
			fmt.Printf("[DEBUG opSetProp] Fallback: TypeDictObject path\n")
		}
		d := AsDictObject(*objVal)
		// Check if property exists
		_, exists := d.GetOwn(propName)
		// Check if we're trying to add a new property to a non-extensible object
		if !exists && !d.IsExtensible() {
			err := vm.NewTypeError(fmt.Sprintf("Cannot add property '%s', object is not extensible", propName))
			if excErr, ok := err.(ExceptionError); ok {
				vm.throwException(excErr.GetExceptionValue())
				return false, InterpretRuntimeError, Undefined
			}
		}
		d.SetOwn(propName, *valueToSet)
	case TypeRegExp:
		// RegExp objects can have user-defined properties
		regex := objVal.AsRegExpObject()
		if regex != nil {
			if regex.Properties == nil {
				regex.Properties = NewObject(Undefined).AsPlainObject()
			}
			regex.Properties.SetOwn(propName, *valueToSet)
		}
		return true, InterpretOK, *valueToSet
	default:
		po := AsPlainObject(*objVal)
		// Check if property exists
		propertyExists := false
		for _, f := range po.shape.fields {
			if f.keyKind == KeyKindString && f.name == propName {
				propertyExists = true
				if !f.writable {
					// Property exists but is not writable - throw TypeError
					err := vm.NewTypeError(fmt.Sprintf("Cannot assign to read only property '%s'", propName))
					if excErr, ok := err.(ExceptionError); ok {
						vm.throwException(excErr.GetExceptionValue())
						return false, InterpretRuntimeError, Undefined
					}
				}
				break
			}
		}
		// Check if we're trying to add a new property to a non-extensible object
		if !propertyExists && !po.IsExtensible() {
			err := vm.NewTypeError(fmt.Sprintf("Cannot add property '%s', object is not extensible", propName))
			if excErr, ok := err.(ExceptionError); ok {
				vm.throwException(excErr.GetExceptionValue())
				return false, InterpretRuntimeError, Undefined
			}
		}
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

	// Handle RegExp objects - they can have symbol properties
	if objVal.Type() == TypeRegExp {
		regex := objVal.AsRegExpObject()
		if regex != nil {
			if regex.Properties == nil {
				regex.Properties = NewObject(Undefined).AsPlainObject()
			}
			key := NewSymbolKey(symKey)
			regex.Properties.DefineOwnPropertyByKey(key, *valueToSet, nil, nil, nil)
		}
		return true, InterpretOK, *valueToSet
	}

	// Only PlainObject supports symbol keys for now
	if objVal.Type() != TypeObject {
		// DictObject or others: ignore symbol set (non-strict semantics)
		if debugVM {
			fmt.Printf("[DBG opSetPropSymbol] Ignoring symbol set on non-PlainObject type=%s\n", objVal.TypeName())
		}
		return true, InterpretOK, *valueToSet
	}

	po := AsPlainObject(*objVal)
	if debugVM {
		fmt.Printf("[DBG opSetPropSymbol] Setting symbol on obj=%p, symbol=%s\n", po, symKey.AsSymbol())
	}

	// Per-site cache keyed by symbol identity
	cacheKey := generateSymbolCacheKey(ip, symKey)
	cache, exists := vm.propCache[cacheKey]
	if !exists {
		cache = &PropInlineCache{state: CacheStateUninitialized}
		vm.propCache[cacheKey] = cache
	}

	// Fast path: cache hit
	symPropName := symKey.ToString() // Use symbol string representation for cache key
	if entry, hit := cache.lookupEntry(po.shape, symPropName); hit {
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
			if debugVM {
				fmt.Printf("[DBG opSetPropSymbol] Updating existing symbol property on obj=%p, symbol=%s, offset=%d, writable=%v, value=%s\n", po, symKey.AsSymbol(), f.offset, f.writable, valueToSet.Inspect())
			}
			if f.writable && f.offset < len(po.properties) {
				po.properties[f.offset] = *valueToSet
			}
			// Update cache with flags
			cache.updateCache(po.shape, symPropName, f.offset, f.isAccessor, f.writable)
			updated = true
			break
		}
	}
	if updated {
		return true, InterpretOK, *valueToSet
	}

	// Define new data property by symbol key with default assignment semantics:
	// writable=true, enumerable=true, configurable=true (per ECMAScript [[Set]])
	if debugVM {
		fmt.Printf("[DBG opSetPropSymbol] Defining new symbol property on obj=%p, symbol=%s, value=%s\n", po, symKey.AsSymbol(), valueToSet.Inspect())
	}
	w, e, c := true, true, true
	po.DefineOwnPropertyByKey(NewSymbolKey(symKey), *valueToSet, &w, &e, &c)
	// Find new field to cache
	for _, f := range po.shape.fields {
		if f.keyKind == KeyKindSymbol && f.symbolVal.obj == symKey.obj {
			cache.updateCache(po.shape, symPropName, f.offset, f.isAccessor, f.writable)
			break
		}
	}
	return true, InterpretOK, *valueToSet
}

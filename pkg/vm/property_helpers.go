package vm

import (
	"unsafe"
)

// handleCallableProperty handles property access on functions and closures
// This consolidates the duplicate logic for these callable types
func (vm *VM) handleCallableProperty(objVal Value, propName string) (Value, bool) {
	var fn *FunctionObject


	switch objVal.Type() {
	case TypeFunction:
		fn = AsFunction(objVal)
	case TypeClosure:
		closure := AsClosure(objVal)
		fn = closure.Fn
	case TypeBoundFunction, TypeNativeFunction, TypeNativeFunctionWithProps, TypeAsyncNativeFunction:
		// Bound/native/async functions inherit from Function.prototype but don't have FunctionObject
		fn = nil
	default:
		return Undefined, false
	}

	// Special handling for "prototype" property (not available on bound functions)
	if fn != nil && propName == "prototype" {
		result := fn.getOrCreatePrototypeWithVM(vm)
		return result, true
	}

	// Other function properties (if any) - not available on bound functions
	if fn != nil && fn.Properties != nil {
		// Check for accessor properties first (getters/setters)
		if getter, _, _, _, exists := fn.Properties.GetOwnAccessor(propName); exists {
			if getter.Type() != TypeUndefined {
				// Call the getter with the function object as 'this'
				res, err := vm.Call(getter, objVal, nil)
				if err != nil {
					// If the getter throws, we need to propagate the exception
					if ee, ok := err.(ExceptionError); ok {
						vm.throwException(ee.GetExceptionValue())
					} else {
						// Wrap non-exception error
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
					}
					// Return undefined; the exception is already set
					return Undefined, false
				}
				return res, true
			}
			// Getter is undefined - return undefined
			return Undefined, true
		}
		// Check for regular data properties
		if prop, exists := fn.Properties.GetOwn(propName); exists {
			return prop, true
		}
	}

	// Expose intrinsic function properties like .name
	if propName == "name" {
		switch objVal.Type() {
		case TypeFunction:
			return NewString(objVal.AsFunction().Name), true
		case TypeClosure:
			return NewString(objVal.AsClosure().Fn.Name), true
		case TypeNativeFunction:
			return NewString(objVal.AsNativeFunction().Name), true
		case TypeAsyncNativeFunction:
			return NewString(objVal.AsAsyncNativeFunction().Name), true
		case TypeNativeFunctionWithProps:
			return NewString(objVal.AsNativeFunctionWithProps().Name), true
		case TypeBoundFunction:
			return NewString(objVal.AsBoundFunction().Name), true
		}
	}

	// Expose intrinsic function properties like .length
	// Per ECMAScript spec, length is the number of parameters before the first default parameter
	if propName == "length" {
		switch objVal.Type() {
		case TypeFunction:
			return NumberValue(float64(objVal.AsFunction().Length)), true
		case TypeClosure:
			return NumberValue(float64(objVal.AsClosure().Fn.Length)), true
		case TypeNativeFunction:
			// For native functions, length is the arity (number of formal parameters)
			// Variadic flag means it accepts additional args, but doesn't affect length
			return NumberValue(float64(objVal.AsNativeFunction().Arity)), true
		case TypeAsyncNativeFunction:
			return NumberValue(float64(objVal.AsAsyncNativeFunction().Arity)), true
		case TypeNativeFunctionWithProps:
			return NumberValue(float64(objVal.AsNativeFunctionWithProps().Arity)), true
		case TypeBoundFunction:
			// Bound functions have reduced length by the number of bound arguments
			bf := objVal.AsBoundFunction()
			// Get the original function's length
			var originalLength int
			switch bf.OriginalFunction.Type() {
			case TypeFunction:
				originalLength = bf.OriginalFunction.AsFunction().Length
			case TypeClosure:
				originalLength = bf.OriginalFunction.AsClosure().Fn.Length
			case TypeNativeFunction:
				originalLength = bf.OriginalFunction.AsNativeFunction().Arity
			case TypeNativeFunctionWithProps:
				originalLength = bf.OriginalFunction.AsNativeFunctionWithProps().Arity
			case TypeAsyncNativeFunction:
				originalLength = bf.OriginalFunction.AsAsyncNativeFunction().Arity
			case TypeBoundFunction:
				// Recursively get the bound function's length
				if length, ok := vm.handleCallableProperty(bf.OriginalFunction, "length"); ok {
					if length.IsNumber() {
						originalLength = int(length.ToFloat())
					}
				}
			}
			// Subtract the number of partial arguments
			boundLength := originalLength - len(bf.PartialArgs)
			if boundLength < 0 {
				boundLength = 0
			}
			return NumberValue(float64(boundLength)), true
		}
	}

	// Expose .constructor on functions to return the Function constructor
	if propName == "constructor" {
		// In JS, Function.prototype.constructor === Function
		// For callable values, return global Function constructor if available
		// Lookup global 'Function' constructor via VM API
		if ctorVal, ok := vm.GetGlobal("Function"); ok && ctorVal.IsCallable() {
			return ctorVal, true
		}
		// Fallback: undefined
		return Undefined, true
	}

	// Expose __proto__ on functions to return Function.prototype
	// This allows Function.prototype.isPrototypeOf(Boolean) to work correctly
	// Special case: Function.prototype itself has Object.prototype as its __proto__
	if propName == "__proto__" {
		if objVal.Is(vm.FunctionPrototype) {
			return vm.ObjectPrototype, true
		}
		return vm.FunctionPrototype, true
	}

	// Check function prototype methods using the VM's FunctionPrototype
	// and walk the prototype chain (Function.prototype -> Object.prototype)
	if vm.FunctionPrototype.Type() == TypeObject {
		funcProto := vm.FunctionPrototype.AsPlainObject()
		if method, exists := funcProto.GetOwn(propName); exists {
			UpdatePrototypeStats("function_proto", 1)
			// Always return the raw method. The VM's OpCallMethod path binds 'this' correctly.
			return method, true
		}
		// Walk prototype chain to Object.prototype
		proto := funcProto.GetPrototype()
		for proto.Type() != TypeNull && proto.Type() != TypeUndefined {
			if proto.Type() == TypeObject {
				if method, exists := proto.AsPlainObject().GetOwn(propName); exists {
					UpdatePrototypeStats("object_proto_via_function", 1)
					return method, true
				}
				proto = proto.AsPlainObject().GetPrototype()
			} else {
				break
			}
		}
	} else if vm.FunctionPrototype.Type() == TypeNativeFunctionWithProps {
		// Function.prototype is a callable NativeFunctionWithProps
		funcProtoObj := vm.FunctionPrototype.AsNativeFunctionWithProps()
		if method, exists := funcProtoObj.Properties.GetOwn(propName); exists {
			UpdatePrototypeStats("function_proto", 1)
			return method, true
		}
		// Walk prototype chain to Object.prototype
		proto := funcProtoObj.Properties.GetPrototype()
		for proto.Type() != TypeNull && proto.Type() != TypeUndefined {
			if proto.Type() == TypeObject {
				if method, exists := proto.AsPlainObject().GetOwn(propName); exists {
					UpdatePrototypeStats("object_proto_via_function", 1)
					return method, true
				}
				proto = proto.AsPlainObject().GetPrototype()
			} else {
				break
			}
		}
	}

	return Undefined, true // Property doesn't exist, but lookup succeeded
}

// handlePrimitiveMethod handles prototype method lookup for primitive types
func (vm *VM) handlePrimitiveMethod(objVal Value, propName string) (Value, bool) {
	// Handle undefined/null objects
	if objVal.Type() == TypeUndefined || objVal.Type() == TypeNull {
		return Undefined, false
	}

	var prototype *PlainObject

	switch objVal.Type() {
	case TypeString:
		prototype = vm.StringPrototype.AsPlainObject()
	case TypeFloatNumber, TypeIntegerNumber:
		if vm.NumberPrototype.Type() == TypeObject {
			prototype = vm.NumberPrototype.AsPlainObject()
		}
	case TypeBoolean:
		// Auto-box primitive boolean to access Boolean.prototype methods
		if vm.BooleanPrototype.Type() == TypeObject {
			prototype = vm.BooleanPrototype.AsPlainObject()
		}
	case TypeArray:
		prototype = vm.ArrayPrototype.AsPlainObject()
	case TypeMap:
		if vm.MapPrototype.Type() == TypeObject {
			prototype = vm.MapPrototype.AsPlainObject()
		}
	case TypeSet:
		if vm.SetPrototype.Type() == TypeObject {
			prototype = vm.SetPrototype.AsPlainObject()
		}
	case TypeRegExp:
		if vm.RegExpPrototype.Type() == TypeObject {
			prototype = vm.RegExpPrototype.AsPlainObject()
		}
	case TypeSymbol:
		if vm.SymbolPrototype.Type() == TypeObject {
			prototype = vm.SymbolPrototype.AsPlainObject()
		}
	case TypeGenerator:
		// Check if generator has a custom prototype, otherwise use default
		genObj := objVal.AsGenerator()
		if genObj.Prototype != nil {
			prototype = genObj.Prototype
		} else if vm.GeneratorPrototype.Type() == TypeObject {
			prototype = vm.GeneratorPrototype.AsPlainObject()
		}
	case TypeAsyncGenerator:
		// Check if async generator has a custom prototype, otherwise use default
		asyncGenObj := objVal.AsAsyncGenerator()
		if asyncGenObj.Prototype != nil {
			prototype = asyncGenObj.Prototype
		} else if vm.AsyncGeneratorPrototype.Type() == TypeObject {
			prototype = vm.AsyncGeneratorPrototype.AsPlainObject()
		}
	case TypePromise:
		if vm.PromisePrototype.Type() == TypeObject {
			prototype = vm.PromisePrototype.AsPlainObject()
		}
	case TypeTypedArray:
		// Get the appropriate typed array prototype based on element type
		ta := objVal.AsTypedArray()
		if ta != nil {
			// Resolve prototype dynamically via global constructors to avoid missing VM fields
			switch ta.GetElementType() {
			case TypedArrayUint8:
				if ctor, ok := vm.GetGlobal("Uint8Array"); ok {
					if ctor.Type() == TypeNativeFunctionWithProps {
						fn := ctor.AsNativeFunctionWithProps()
						if p, hit := fn.Properties.GetOwn("prototype"); hit {
							prototype = p.AsPlainObject()
						}
					}
				}
			case TypedArrayUint8Clamped:
				if ctor, ok := vm.GetGlobal("Uint8ClampedArray"); ok {
					if ctor.Type() == TypeNativeFunctionWithProps {
						fn := ctor.AsNativeFunctionWithProps()
						if p, hit := fn.Properties.GetOwn("prototype"); hit {
							prototype = p.AsPlainObject()
						}
					}
				}
			case TypedArrayInt8:
				if ctor, ok := vm.GetGlobal("Int8Array"); ok {
					if ctor.Type() == TypeNativeFunctionWithProps {
						fn := ctor.AsNativeFunctionWithProps()
						if p, hit := fn.Properties.GetOwn("prototype"); hit {
							prototype = p.AsPlainObject()
						}
					}
				}
			case TypedArrayInt16:
				if ctor, ok := vm.GetGlobal("Int16Array"); ok {
					if ctor.Type() == TypeNativeFunctionWithProps {
						fn := ctor.AsNativeFunctionWithProps()
						if p, hit := fn.Properties.GetOwn("prototype"); hit {
							prototype = p.AsPlainObject()
						}
					}
				}
			case TypedArrayUint16:
				if ctor, ok := vm.GetGlobal("Uint16Array"); ok {
					if ctor.Type() == TypeNativeFunctionWithProps {
						fn := ctor.AsNativeFunctionWithProps()
						if p, hit := fn.Properties.GetOwn("prototype"); hit {
							prototype = p.AsPlainObject()
						}
					}
				}
			case TypedArrayUint32:
				if ctor, ok := vm.GetGlobal("Uint32Array"); ok {
					if ctor.Type() == TypeNativeFunctionWithProps {
						fn := ctor.AsNativeFunctionWithProps()
						if p, hit := fn.Properties.GetOwn("prototype"); hit {
							prototype = p.AsPlainObject()
						}
					}
				}
			case TypedArrayInt32:
				if ctor, ok := vm.GetGlobal("Int32Array"); ok {
					if ctor.Type() == TypeNativeFunctionWithProps {
						fn := ctor.AsNativeFunctionWithProps()
						if p, hit := fn.Properties.GetOwn("prototype"); hit {
							prototype = p.AsPlainObject()
						}
					}
				}
			case TypedArrayFloat32:
				if ctor, ok := vm.GetGlobal("Float32Array"); ok {
					if ctor.Type() == TypeNativeFunctionWithProps {
						fn := ctor.AsNativeFunctionWithProps()
						if p, hit := fn.Properties.GetOwn("prototype"); hit {
							prototype = p.AsPlainObject()
						}
					}
				}
			case TypedArrayFloat64:
				if ctor, ok := vm.GetGlobal("Float64Array"); ok {
					if ctor.Type() == TypeNativeFunctionWithProps {
						fn := ctor.AsNativeFunctionWithProps()
						if p, hit := fn.Properties.GetOwn("prototype"); hit {
							prototype = p.AsPlainObject()
						}
					}
				}
			}
		}
	default:
		return Undefined, false
	}

	if prototype != nil {
		if method, exists := prototype.GetOwn(propName); exists {
			if EnableDetailedCacheStats {
				UpdatePrototypeStats("primitive_method", 0)
			}
			// Return raw method so caller can supply correct 'this' (works for both o.m() and borrowed calls)
			return method, true
		}
	}

	return Undefined, false
}

// handleSpecialProperties handles special properties like .length
func (vm *VM) handleSpecialProperties(objVal Value, propName string) (Value, bool) {
	// Handle undefined/null objects
	if objVal.Type() == TypeUndefined || objVal.Type() == TypeNull {
		return Undefined, false
	}

	if propName == "length" {
		switch objVal.Type() {
		case TypeArray:
			arr := AsArray(objVal)
			return Number(float64(arr.Length())), true
		case TypeArguments:
			args := AsArguments(objVal)
			return Number(float64(args.Length())), true
		case TypeString:
			str := AsString(objVal)
			// Use UTF-16 code unit count for correct JavaScript string length
			return Number(float64(UTF16Length(str))), true
		}
	}
	if propName == "callee" {
		switch objVal.Type() {
		case TypeArguments:
			args := AsArguments(objVal)
			return args.callee, true
		}
	}

	if propName == "size" {
		switch objVal.Type() {
		case TypeMap:
			mapObj := AsMap(objVal)
			return Number(float64(mapObj.Size())), true
		case TypeSet:
			setObj := AsSet(objVal)
			return Number(float64(setObj.Size())), true
		}
	}

	// Handle RegExp properties
	if objVal.Type() == TypeRegExp {
		regexObj := objVal.AsRegExpObject()
		if regexObj != nil {
			switch propName {
			case "source":
				return NewString(regexObj.GetSource()), true
			case "flags":
				return NewString(regexObj.GetFlags()), true
			case "global":
				return BooleanValue(regexObj.IsGlobal()), true
			case "ignoreCase":
				return BooleanValue(regexObj.IsIgnoreCase()), true
			case "multiline":
				return BooleanValue(regexObj.IsMultiline()), true
			case "dotAll":
				return BooleanValue(regexObj.IsDotAll()), true
			case "lastIndex":
				return Number(float64(regexObj.GetLastIndex())), true
			}
		}
	}

	// Handle ArrayBuffer properties
	if objVal.Type() == TypeArrayBuffer {
		buffer := objVal.AsArrayBuffer()
		if buffer != nil {
			switch propName {
			case "byteLength":
				return Number(float64(len(buffer.GetData()))), true
			}
		}
	}

	// Handle TypedArray properties
	if objVal.Type() == TypeTypedArray {
		ta := objVal.AsTypedArray()
		if ta != nil {
			switch propName {
			case "length":
				return Number(float64(ta.GetLength())), true
			case "byteLength":
				return Number(float64(ta.GetByteLength())), true
			case "byteOffset":
				return Number(float64(ta.GetByteOffset())), true
			case "buffer":
				return Value{typ: TypeArrayBuffer, obj: unsafe.Pointer(ta.GetBuffer())}, true
			case "BYTES_PER_ELEMENT":
				return Number(float64(ta.GetBytesPerElement())), true
			}
		}
	}

	return Undefined, false
}

// traversePrototypeChain walks up the prototype chain looking for a property
func (vm *VM) traversePrototypeChain(obj *PlainObject, propName string, cacheKey int) (Value, int, bool) {
	depth := 0
	current := obj

	for current != nil && depth < 10 { // Prevent infinite loops
		// Check own properties
		if val, exists := current.GetOwn(propName); exists {
			return val, depth, true
		}

		// Move up the prototype chain
		protoVal := current.GetPrototype()
		if !protoVal.IsObject() {
			break
		}

		current = protoVal.AsPlainObject()
		depth++
	}

	return Undefined, 0, false
}

// resolvePropertyWithCache performs cached property resolution with prototype chain support
func (vm *VM) resolvePropertyWithCache(objVal Value, propName string, cache *PropInlineCache, cacheKey int) (Value, bool) {
	// Check if we have prototype caching enabled
	var protoCache *PrototypeCache
	if EnablePrototypeCache {
		protoCache = GetOrCreatePrototypeCache(cacheKey)
	}

	// For PlainObjects, check both regular cache and prototype cache
	if objVal.Type() == TypeObject {
		po := AsPlainObject(objVal)

		// Check prototype cache first if enabled
		if protoCache != nil {
			if entry, hit := protoCache.Lookup(po.shape); hit {
				UpdatePrototypeStats("proto_hit", entry.prototypeDepth)

				if entry.isMethod && entry.boundMethod.Type() != TypeUndefined {
					UpdatePrototypeStats("bound_method_cached", 0)
					return entry.boundMethod, true
				}

				// Property found in cached prototype
				if entry.prototypeObj != nil && entry.offset < len(entry.prototypeObj.properties) {
					// Also update inline cache with proto holder guard
					if cache != nil {
						isAcc := false
						if entry.offset < len(entry.prototypeObj.shape.fields) {
							isAcc = entry.prototypeObj.shape.fields[entry.offset].isAccessor
						}
						cache.updateCacheProto(po.shape, entry.prototypeObj.shape, entry.offset, int8(entry.prototypeDepth), isAcc)
					}
					return entry.prototypeObj.properties[entry.offset], true
				}
			}
		}

		// Fall back to traversing prototype chain
		if val, depth, found := vm.traversePrototypeChain(po, propName, cacheKey); found {
			// Cache the result if prototype caching is enabled
			if protoCache != nil && depth > 0 {
				// Find the prototype object where property was found
				current := po
				for i := 0; i < depth && current != nil; i++ {
					protoVal := current.GetPrototype()
					if protoVal.IsObject() {
						current = protoVal.AsPlainObject()
					}
				}

				if current != nil {
					// Find offset in prototype
					offset := -1
					for _, field := range current.shape.fields {
						if field.name == propName {
							offset = field.offset
							break
						}
					}

					if offset >= 0 {
						protoCache.Update(po.shape, current, depth, offset, Undefined, false)
						if cache != nil {
							isAcc := false
							if offset < len(current.shape.fields) {
								isAcc = current.shape.fields[offset].isAccessor
							}
							cache.updateCacheProto(po.shape, current.shape, offset, int8(depth), isAcc)
						}
					}
				}
			}

			if EnableDetailedCacheStats && depth > 0 {
				UpdatePrototypeStats("proto_hit", depth)
			}

			return val, true
		}

		if EnableDetailedCacheStats {
			UpdatePrototypeStats("proto_miss", 0)
		}
	}

	return Undefined, false
}

// resolvePropertyMeta resolves a property and returns holder object, offset within holder, and accessor flag.
// It also updates the inline cache (and prototype cache when enabled) similarly to resolvePropertyWithCache.
func (vm *VM) resolvePropertyMeta(objVal Value, propName string, cache *PropInlineCache, cacheKey int) (*PlainObject, int, bool, bool) {
	// Only meaningful for PlainObject
	if objVal.Type() != TypeObject {
		return nil, -1, false, false
	}

	po := AsPlainObject(objVal)

	// 1) Own property fast detection via shape scan
	for _, f := range po.shape.fields {
		if f.keyKind == KeyKindString && f.name == propName {
			// Inline cache update for own property happens at caller
			return po, f.offset, f.isAccessor, true
		}
	}

	// 2) Prototype cache lookup when enabled
	var protoCache *PrototypeCache
	if EnablePrototypeCache {
		protoCache = GetOrCreatePrototypeCache(cacheKey)
	}
	if protoCache != nil {
		if entry, hit := protoCache.Lookup(po.shape); hit {
			UpdatePrototypeStats("proto_hit", entry.prototypeDepth)
			if entry.prototypeObj != nil && entry.offset < len(entry.prototypeObj.properties) {
				if cache != nil {
					isAcc := false
					if entry.offset < len(entry.prototypeObj.shape.fields) {
						isAcc = entry.prototypeObj.shape.fields[entry.offset].isAccessor
					}
					cache.updateCacheProto(po.shape, entry.prototypeObj.shape, entry.offset, int8(entry.prototypeDepth), isAcc)
				}
				return entry.prototypeObj, entry.offset, entry.prototypeObj.shape.fields[entry.offset].isAccessor, true
			}
		}
	}

	// 3) Traverse prototype chain
	depth := 0
	current := po
	for current != nil && depth < 10 {
		// Already checked own properties of base; skip this iteration for depth 0
		if depth > 0 {
			for _, f := range current.shape.fields {
				if f.keyKind == KeyKindString && f.name == propName {
					// Update caches
					if protoCache != nil {
						protoCache.Update(po.shape, current, depth, f.offset, Undefined, false)
					}
					if cache != nil {
						cache.updateCacheProto(po.shape, current.shape, f.offset, int8(depth), f.isAccessor)
					}
					return current, f.offset, f.isAccessor, true
				}
			}
		}
		pv := current.GetPrototype()
		if !pv.IsObject() {
			break
		}
		current = pv.AsPlainObject()
		depth++
	}

	if EnableDetailedCacheStats {
		UpdatePrototypeStats("proto_miss", 0)
	}
	return nil, -1, false, false
}

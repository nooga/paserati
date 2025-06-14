package vm

import "unicode/utf8"

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
	default:
		return Undefined, false
	}

	// Special handling for "prototype" property
	if propName == "prototype" {
		return fn.getOrCreatePrototype(), true
	}

	// Other function properties (if any)
	if fn.Properties != nil {
		if prop, exists := fn.Properties.GetOwn(propName); exists {
			return prop, true
		}
	}

	// Check function prototype methods using the VM's FunctionPrototype
	if vm.FunctionPrototype.Type() == TypeObject {
		funcProto := vm.FunctionPrototype.AsPlainObject()
		if method, exists := funcProto.GetOwn(propName); exists {
			UpdatePrototypeStats("function_proto", 1)

			// Special handling for Function.prototype.call and apply to prevent infinite recursion
			// These methods need special treatment because they create bound methods that would
			// recursively call themselves when accessed through property lookup
			if propName == "call" || propName == "apply" {
				// Return the raw method without binding - the method implementation
				// will handle the 'this' binding internally
				return method, true
			}

			return createBoundMethod(objVal, method), true
		}
	}

	return Undefined, true // Property doesn't exist, but lookup succeeded
}

// handlePrimitiveMethod handles prototype method lookup for primitive types
func (vm *VM) handlePrimitiveMethod(objVal Value, propName string) (Value, bool) {
	var prototype *PlainObject

	switch objVal.Type() {
	case TypeString:
		prototype = vm.StringPrototype.AsPlainObject()
	case TypeArray:
		prototype = vm.ArrayPrototype.AsPlainObject()
	default:
		return Undefined, false
	}

	if prototype != nil {
		if method, exists := prototype.GetOwn(propName); exists {
			// Track primitive method hits if detailed stats enabled
			if EnableDetailedCacheStats {
				UpdatePrototypeStats("primitive_method", 0)
			}
			return createBoundMethod(objVal, method), true
		}
	}

	return Undefined, false
}

// handleSpecialProperties handles special properties like .length
func (vm *VM) handleSpecialProperties(objVal Value, propName string) (Value, bool) {
	if propName == "length" {
		switch objVal.Type() {
		case TypeArray:
			arr := AsArray(objVal)
			return Number(float64(len(arr.elements))), true
		case TypeString:
			str := AsString(objVal)
			// Use rune count for correct length of multi-byte strings
			return Number(float64(utf8.RuneCountInString(str))), true
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

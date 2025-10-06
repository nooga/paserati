package checker

import (
	"fmt"
	"paserati/pkg/parser"
	"paserati/pkg/types"
	"paserati/pkg/vm"
)

func (c *Checker) checkArrayLiteral(node *parser.ArrayLiteral) {
	generalizedElementTypes := []types.Type{} // Store generalized types
	for _, elemNode := range node.Elements {
		c.visit(elemNode) // Visit element to compute its type
		elemType := elemNode.GetComputedType()
		if elemType == nil {
			elemType = types.Any
		} // Handle error

		// Handle spread elements specially
		if spreadElem, isSpread := elemNode.(*parser.SpreadElement); isSpread {
			// For spread elements, extract the element type from the array
			if arrayType, isArray := elemType.(*types.ArrayType); isArray {
				// Use the element type of the spread array
				generalizedType := types.DeeplyWidenType(arrayType.ElementType)
				generalizedElementTypes = append(generalizedElementTypes, generalizedType)
			} else if elemType == types.Any {
				// Spread of 'any' contributes 'any' element type
				generalizedElementTypes = append(generalizedElementTypes, types.Any)
			} else {
				// Error case - spread of non-array: issue a clear compile error
				c.addError(spreadElem, "spread syntax can only be applied to arrays")
				generalizedElementTypes = append(generalizedElementTypes, types.Any)
			}
		} else {
			// Regular element
			generalizedType := types.DeeplyWidenType(elemType)
			generalizedElementTypes = append(generalizedElementTypes, generalizedType)
		}
	}

	// Determine the element type for the array using the GENERALIZED types.
	var finalElementType types.Type
	if len(generalizedElementTypes) == 0 {
		finalElementType = types.Unknown
	} else {
		// Use NewUnionType to flatten/uniquify GENERALIZED element types
		finalElementType = types.NewUnionType(generalizedElementTypes...)
		// NewUnionType should simplify if all generalized types are identical
	}

	// Create the ArrayType
	arrayType := &types.ArrayType{ElementType: finalElementType}

	// Set the computed type for the ArrayLiteral node itself
	node.SetComputedType(arrayType)

	debugPrintf("// [Checker ArrayLit] Computed ElementType: %s, Full ArrayType: %s\n", finalElementType.String(), arrayType.String())
}

// checkArrayLiteralWithContext checks array literals with contextual type information
func (c *Checker) checkArrayLiteralWithContext(node *parser.ArrayLiteral, context *ContextualType) {
	expectedType := context.ExpectedType
	debugPrintf("// [Checker ArrayLitContext] Expected type: %T (%s)\n", expectedType, expectedType.String())

	// Check if expected type is a tuple type
	if tupleType, isTuple := expectedType.(*types.TupleType); isTuple {
		debugPrintf("// [Checker ArrayLitContext] Expected tuple with %d element types\n", len(tupleType.ElementTypes))

		// For tuple context, check if we have enough elements
		// Count required (non-optional) elements
		minRequired := 0
		for i := 0; i < len(tupleType.ElementTypes); i++ {
			if tupleType.OptionalElements == nil || i >= len(tupleType.OptionalElements) || !tupleType.OptionalElements[i] {
				minRequired = i + 1
			}
		}

		hasRestElement := tupleType.RestElementType != nil
		maxAllowed := len(tupleType.ElementTypes)
		if hasRestElement {
			maxAllowed = -1 // No upper limit with rest element
		}

		// Check element count
		if len(node.Elements) < minRequired {
			debugPrintf("// [Checker ArrayLitContext] Not enough elements: expected at least %d, got %d. Using regular array checking.\n", minRequired, len(node.Elements))
			c.checkArrayLiteral(node)
			return
		}
		if maxAllowed >= 0 && len(node.Elements) > maxAllowed {
			debugPrintf("// [Checker ArrayLitContext] Too many elements: expected at most %d, got %d. Using regular array checking.\n", maxAllowed, len(node.Elements))
			c.checkArrayLiteral(node)
			return
		}

		// Check each element against the corresponding tuple element type
		elementTypesMatch := true
		for i, elemNode := range node.Elements {
			var expectedElemType types.Type

			if i < len(tupleType.ElementTypes) {
				// Fixed element
				expectedElemType = tupleType.ElementTypes[i]
			} else if hasRestElement {
				// Rest element - for [...number[]], each rest element should be number
				if arrayType, ok := tupleType.RestElementType.(*types.ArrayType); ok {
					expectedElemType = arrayType.ElementType
				} else {
					// Fallback if rest element is not an array type
					expectedElemType = tupleType.RestElementType
				}
			} else {
				// Should not happen - we checked count above
				elementTypesMatch = false
				break
			}

			// Use contextual typing for each element
			c.visitWithContext(elemNode, &ContextualType{
				ExpectedType: expectedElemType,
				IsContextual: true,
			})

			actualElemType := elemNode.GetComputedType()
			if actualElemType == nil {
				actualElemType = types.Any
			}

			// Check if the element type is assignable to the expected tuple element type
			if !types.IsAssignable(actualElemType, expectedElemType) {
				elementTypesMatch = false
				debugPrintf("// [Checker ArrayLitContext] Element %d type mismatch: expected %s, got %s\n", i, expectedElemType.String(), actualElemType.String())
			}
		}

		if elementTypesMatch {
			// All elements match - use the tuple type
			node.SetComputedType(tupleType)
			debugPrintf("// [Checker ArrayLitContext] All elements match tuple. Set type to: %s\n", tupleType.String())
			return
		} else {
			// Elements don't match - fall back to regular array checking but don't return error
			// (the regular checking will handle type errors)
			debugPrintf("// [Checker ArrayLitContext] Element types don't match tuple. Falling back to regular array checking.\n")
			c.checkArrayLiteral(node)
			return
		}
	}

	// Check if expected type is an array type
	if arrayType, isArray := expectedType.(*types.ArrayType); isArray {
		debugPrintf("// [Checker ArrayLitContext] Expected array with element type: %s\n", arrayType.ElementType.String())

		// Check each element against the expected element type
		for _, elemNode := range node.Elements {
			// Handle spread elements specially
			if _, isSpread := elemNode.(*parser.SpreadElement); isSpread {
				// For spread elements, visit without context first to get the spread type
				c.visit(elemNode)
				spreadType := elemNode.GetComputedType()
				if spreadType == nil {
					spreadType = types.Any
				}

				// Validate that the spread array's element type is assignable to expected element type
				if spreadArrayType, isArray := spreadType.(*types.ArrayType); isArray {
					if !types.IsAssignable(spreadArrayType.ElementType, arrayType.ElementType) {
						c.addError(elemNode, fmt.Sprintf("Type '%s' is not assignable to type '%s'",
							spreadArrayType.ElementType.String(), arrayType.ElementType.String()))
					}
				} else if spreadType != types.Any {
					// Spread of non-array type (error should be caught elsewhere)
					c.addError(elemNode, fmt.Sprintf("Type '%s' is not assignable to type '%s'",
						spreadType.String(), arrayType.ElementType.String()))
				}
			} else {
				// Regular element
				c.visitWithContext(elemNode, &ContextualType{
					ExpectedType: arrayType.ElementType,
					IsContextual: true,
				})

				// Validate that the element is assignable to the expected element type
				actualElemType := elemNode.GetComputedType()
				if actualElemType == nil {
					actualElemType = types.Any
				}

				if !types.IsAssignable(actualElemType, arrayType.ElementType) {
					c.addError(elemNode, fmt.Sprintf("Type '%s' is not assignable to type '%s'",
						actualElemType.String(), arrayType.ElementType.String()))
				}
			}
		}

		// Use the expected array type as the result (even if there are errors)
		node.SetComputedType(arrayType)
		debugPrintf("// [Checker ArrayLitContext] Set type to expected array type: %s\n", arrayType.String())
		return
	}

	// For other expected types, fall back to regular array literal checking
	debugPrintf("// [Checker ArrayLitContext] Expected type is not array or tuple (%T). Using regular array checking.\n", expectedType)
	c.checkArrayLiteral(node)
}

// checkObjectLiteral checks the type of an object literal expression.
func (c *Checker) checkObjectLiteral(node *parser.ObjectLiteral) {
	fields := make(map[string]types.Type)
	seenKeys := make(map[string]bool)

	// --- NEW: Create preliminary object type for 'this' context ---
	// We need to construct the object type first so function methods can reference it
	preliminaryObjType := &types.ObjectType{Properties: make(map[string]types.Type)}

	// First pass: collect all non-function properties AND create preliminary function signatures
	for _, prop := range node.Properties {
		var keyName string

		switch key := prop.Key.(type) {
		case *parser.Identifier:
			keyName = key.Value
		case *parser.StringLiteral:
			keyName = key.Value
		case *parser.NumberLiteral: // Allow number literals as keys, convert to string
			// Note: JavaScript converts number keys to strings internally
			keyName = fmt.Sprintf("%v", key.Value) // Simple conversion
		case *parser.SpreadElement:
			// Handle spread syntax: {...obj}
			// Check that the argument is an object type
			c.visit(key.Argument)
			argType := key.Argument.GetComputedType()
			if argType == nil {
				argType = types.Any
			}

			// Check if the type can be spread (is an object type)
			widenedType := types.GetWidenedType(argType)
			switch spreadObjType := widenedType.(type) {
			case *types.ObjectType:
				// Valid object type for spreading - merge its properties
				debugPrintf("// [Checker ObjectLit Spread] Merging properties from spread object: %s\n", spreadObjType.String())
				for propName, propType := range spreadObjType.Properties {
					fields[propName] = propType // Later properties override earlier ones
					debugPrintf("// [Checker ObjectLit Spread] Added property '%s': %s\n", propName, propType.String())
				}
			case *types.ArrayType:
				// Arrays can be spread but only add numeric indices and length
				// For simplicity, we'll allow this but not add specific properties
				debugPrintf("// [Checker ObjectLit Spread] Spreading array type (no properties added)\n")
			default:
				if widenedType != types.Any {
					c.addError(key.Argument, fmt.Sprintf("spread syntax requires an object, got %s", argType.String()))
				}
			}
			// Skip the rest of the property processing for spread elements
			continue
		case *parser.ComputedPropertyName:
			// Handle computed properties: [expression]
			c.visit(key.Expr)
			keyType := key.Expr.GetComputedType()
			if keyType == nil {
				keyType = types.Any
			}

			// Try to get a compile-time constant key if possible
			if literal, ok := key.Expr.(*parser.StringLiteral); ok {
				keyName = literal.Value
			} else if literal, ok := key.Expr.(*parser.NumberLiteral); ok {
				keyName = fmt.Sprintf("%v", literal.Value)
			} else if memberExpr, ok := key.Expr.(*parser.MemberExpression); ok {
				// Check for Symbol.iterator access
				if objectIdent, ok := memberExpr.Object.(*parser.Identifier); ok {
					if objectIdent.Value == "Symbol" {
						if propertyIdent, ok := memberExpr.Property.(*parser.Identifier); ok {
							if propertyIdent.Value == "iterator" {
								// This is Symbol.iterator - treat as computed symbol key (no stringization)
								keyName = "__COMPUTED_PROPERTY__"
							} else {
								// Other Symbol properties - use generic @@symbol: prefix
								keyName = "@@symbol:" + propertyIdent.Value
							}
						} else {
							// Complex Symbol property access
							keyName = "__COMPUTED_PROPERTY__"
						}
					} else {
						// Non-Symbol member expression
						keyName = "__COMPUTED_PROPERTY__"
					}
				} else {
					// Complex computed property expression - mark for index signature
					keyName = "__COMPUTED_PROPERTY__"
				}
			} else {
				// Complex computed property expression - mark for index signature
				keyName = "__COMPUTED_PROPERTY__"
			}
		default:
			// Unsupported key type
			c.addError(prop.Key, fmt.Sprintf("unsupported object literal key type: %T", prop.Key))
			keyName = "__UNKNOWN_KEY__"
		}

		// Check for duplicate keys (but skip for computed keys since they can be dynamic)
		// Allow getters/setters to share the same key by checking if the value is a MethodDefinition
		if keyName != "__COMPUTED_PROPERTY__" && keyName != "__UNKNOWN_KEY__" {
			// Check if this is a getter/setter MethodDefinition
			if methodDef, isMethodDef := prop.Value.(*parser.MethodDefinition); isMethodDef {
				if methodDef.Kind == "getter" || methodDef.Kind == "setter" {
					// Getters and setters can share the same key name - don't mark as duplicate
					// But do track them to prevent multiple getters or multiple setters for same key
					// For now, be lenient and allow it - JavaScript allows redefining getters/setters
				} else {
					// Regular method - check for conflicts
					if seenKeys[keyName] {
						c.addError(prop.Key, fmt.Sprintf("duplicate property key: '%s'", keyName))
					}
					seenKeys[keyName] = true
				}
			} else {
				// Regular property - check for conflicts
				if seenKeys[keyName] {
					c.addError(prop.Key, fmt.Sprintf("duplicate property key: '%s'", keyName))
				}
				seenKeys[keyName] = true
			}
		}

		// For non-function properties, visit and store the type immediately
		if _, isFunctionLiteral := prop.Value.(*parser.FunctionLiteral); !isFunctionLiteral {
			if _, isArrowFunction := prop.Value.(*parser.ArrowFunctionLiteral); !isArrowFunction {
				if _, isShorthandMethod := prop.Value.(*parser.ShorthandMethod); !isShorthandMethod {
					if _, isMethodDef := prop.Value.(*parser.MethodDefinition); !isMethodDef {
						// Visit the value to determine its type
						c.visit(prop.Value)
						valueType := prop.Value.GetComputedType()
						if valueType == nil {
							// If visiting the value failed, checker should have added error.
							// Default to Any to prevent cascading nil errors.
							valueType = types.Any
						}

						// Special handling for __proto__: merge prototype properties
						if keyName == "__proto__" {
							// __proto__ sets the prototype, so merge its properties into the object type
							widenedProtoType := types.GetWidenedType(valueType)
							if protoObjType, ok := widenedProtoType.(*types.ObjectType); ok {
								// Merge prototype properties into our object
								for propName, propType := range protoObjType.Properties {
									// Only add if not already defined (own properties override prototype)
									if _, exists := fields[propName]; !exists {
										fields[propName] = propType
										preliminaryObjType.Properties[propName] = propType
									}
								}
							}
							// Don't add __proto__ itself as a property
						} else {
							fields[keyName] = valueType
							preliminaryObjType.Properties[keyName] = valueType
						}
					}
				}
			}
		}
	}

	// Second pass: create preliminary function signatures for all function properties
	for _, prop := range node.Properties {
		var keyName string

		switch key := prop.Key.(type) {
		case *parser.Identifier:
			keyName = key.Value
		case *parser.StringLiteral:
			keyName = key.Value
		case *parser.NumberLiteral:
			keyName = fmt.Sprintf("%v", key.Value)
		case *parser.SpreadElement:
			continue // Skip spread elements
		case *parser.ComputedPropertyName:
			// Handle computed properties in the same way as first pass
			if literal, ok := key.Expr.(*parser.StringLiteral); ok {
				keyName = literal.Value
			} else if literal, ok := key.Expr.(*parser.NumberLiteral); ok {
				keyName = fmt.Sprintf("%v", literal.Value)
			} else if memberExpr, ok := key.Expr.(*parser.MemberExpression); ok {
				// Check for Symbol.iterator access
				if objectIdent, ok := memberExpr.Object.(*parser.Identifier); ok {
					if objectIdent.Value == "Symbol" {
						if propertyIdent, ok := memberExpr.Property.(*parser.Identifier); ok {
							if propertyIdent.Value == "iterator" {
								// This is Symbol.iterator - treat as computed symbol key
								keyName = "__COMPUTED_PROPERTY__"
							} else {
								// Other Symbol properties - use generic @@symbol: prefix
								keyName = "@@symbol:" + propertyIdent.Value
							}
						} else {
							// Complex Symbol property access
							keyName = "__COMPUTED_PROPERTY__"
						}
					} else {
						// Non-Symbol member expression
						keyName = "__COMPUTED_PROPERTY__"
					}
				} else {
					// Complex computed property expression - mark for index signature
					keyName = "__COMPUTED_PROPERTY__"
				}
			} else {
				// Complex computed property expression - mark for index signature
				keyName = "__COMPUTED_PROPERTY__"
			}
		default:
			// Unsupported key type
			c.addError(prop.Key, fmt.Sprintf("unsupported object literal key type: %T", prop.Key))
			keyName = "__UNKNOWN_KEY__"
		}

		// Skip if already processed in first pass
		if _, alreadyProcessed := fields[keyName]; alreadyProcessed {
			continue
		}

		// Create preliminary function signatures for function properties
		if funcLit, isFunctionLiteral := prop.Value.(*parser.FunctionLiteral); isFunctionLiteral {
			preliminaryFuncSig := c.resolveFunctionLiteralSignature(funcLit, c.env)
			if preliminaryFuncSig == nil {
				preliminaryFuncSig = &types.Signature{
					ParameterTypes: make([]types.Type, len(funcLit.Parameters)),
					ReturnType:     types.Any,
				}
				for i := range preliminaryFuncSig.ParameterTypes {
					preliminaryFuncSig.ParameterTypes[i] = types.Any
				}
			}
			preliminaryObjType.Properties[keyName] = types.NewFunctionType(preliminaryFuncSig)
		} else if _, isArrowFunction := prop.Value.(*parser.ArrowFunctionLiteral); isArrowFunction {
			// For arrow functions, we can use a generic function type temporarily
			preliminaryObjType.Properties[keyName] = types.NewFunctionType(&types.Signature{
				ParameterTypes: []types.Type{}, // We'll refine this later
				ReturnType:     types.Any,
			})
		} else if shorthandMethod, isShorthandMethod := prop.Value.(*parser.ShorthandMethod); isShorthandMethod {
			// Create preliminary function signature for shorthand methods
			paramTypes := make([]types.Type, len(shorthandMethod.Parameters))
			for i, param := range shorthandMethod.Parameters {
				if param.TypeAnnotation != nil {
					paramType := c.resolveTypeAnnotation(param.TypeAnnotation)
					if paramType != nil {
						paramTypes[i] = paramType
					} else {
						paramTypes[i] = types.Any
					}
				} else {
					paramTypes[i] = types.Any
				}
			}

			var returnType types.Type
			if shorthandMethod.ReturnTypeAnnotation != nil {
				returnType = c.resolveTypeAnnotation(shorthandMethod.ReturnTypeAnnotation)
			}
			if returnType == nil {
				returnType = types.Any // We'll infer this later during body checking
			}

			preliminaryFuncSig := &types.Signature{
				ParameterTypes: paramTypes,
				ReturnType:     returnType,
			}
			preliminaryObjType.Properties[keyName] = types.NewFunctionType(preliminaryFuncSig)
		} else if methodDef, isMethodDef := prop.Value.(*parser.MethodDefinition); isMethodDef {
			// Create preliminary function signature for method definitions (getters/setters)
			if methodDef.Value != nil {
				funcLit := methodDef.Value // Already a *parser.FunctionLiteral
				preliminaryFuncSig := c.resolveFunctionLiteralSignature(funcLit, c.env)
				if preliminaryFuncSig == nil {
					preliminaryFuncSig = &types.Signature{
						ParameterTypes: make([]types.Type, len(funcLit.Parameters)),
						ReturnType:     types.Any,
					}
					for i := range preliminaryFuncSig.ParameterTypes {
						preliminaryFuncSig.ParameterTypes[i] = types.Any
					}
				}

				// Store with appropriate prefix for the second pass too
				if methodDef.Kind == "getter" {
					getterName := "__get__" + keyName
					preliminaryObjType.Properties[getterName] = types.NewFunctionType(preliminaryFuncSig)
					// Also store the property type for type checking
					preliminaryObjType.Properties[keyName] = preliminaryFuncSig.ReturnType
				} else if methodDef.Kind == "setter" {
					setterName := "__set__" + keyName
					preliminaryObjType.Properties[setterName] = types.NewFunctionType(preliminaryFuncSig)
					// Also store the property type for type checking
					if len(preliminaryFuncSig.ParameterTypes) > 0 {
						preliminaryObjType.Properties[keyName] = preliminaryFuncSig.ParameterTypes[0]
					} else {
						preliminaryObjType.Properties[keyName] = types.Any
					}
				} else {
					preliminaryObjType.Properties[keyName] = types.NewFunctionType(preliminaryFuncSig)
				}
			}
		}
	}

	// Third pass: visit function properties with 'this' context set to the complete preliminary type
	outerThisType := c.currentThisType     // Save outer this context
	c.currentThisType = preliminaryObjType // Set this context to the object being constructed
	debugPrintf("// [Checker ObjectLit] Set this context to: %s\n", preliminaryObjType.String())

	for _, prop := range node.Properties {
		var keyName string

		switch key := prop.Key.(type) {
		case *parser.Identifier:
			keyName = key.Value
		case *parser.StringLiteral:
			keyName = key.Value
		case *parser.NumberLiteral:
			keyName = fmt.Sprintf("%v", key.Value)
		case *parser.SpreadElement:
			continue // Skip spread elements
		case *parser.ComputedPropertyName:
			// Handle computed properties in the same way as previous passes
			if literal, ok := key.Expr.(*parser.StringLiteral); ok {
				keyName = literal.Value
			} else if literal, ok := key.Expr.(*parser.NumberLiteral); ok {
				keyName = fmt.Sprintf("%v", literal.Value)
			} else if memberExpr, ok := key.Expr.(*parser.MemberExpression); ok {
				// Check for Symbol.iterator access
				if objectIdent, ok := memberExpr.Object.(*parser.Identifier); ok {
					if objectIdent.Value == "Symbol" {
						if propertyIdent, ok := memberExpr.Property.(*parser.Identifier); ok {
							if propertyIdent.Value == "iterator" {
								// This is Symbol.iterator - treat as computed symbol key
								keyName = "__COMPUTED_PROPERTY__"
							} else {
								// Other Symbol properties - use generic @@symbol: prefix
								keyName = "@@symbol:" + propertyIdent.Value
							}
						} else {
							// Complex Symbol property access
							keyName = "__COMPUTED_PROPERTY__"
						}
					} else {
						// Non-Symbol member expression
						keyName = "__COMPUTED_PROPERTY__"
					}
				} else {
					// Complex computed property expression - mark for index signature
					keyName = "__COMPUTED_PROPERTY__"
				}
			} else {
				// Complex computed property expression - mark for index signature
				keyName = "__COMPUTED_PROPERTY__"
			}
		default:
			// Unsupported key type
			keyName = "__UNKNOWN_KEY__"
		}

		// Skip if already processed in first pass (non-function properties)
		if _, isNonFunction := fields[keyName]; isNonFunction {
			continue
		}

		// Visit function properties with 'this' context
		if _, isFunctionLiteral := prop.Value.(*parser.FunctionLiteral); isFunctionLiteral {
			debugPrintf("// [Checker ObjectLit] Visiting function property '%s' with this context\n", keyName)
			c.visit(prop.Value)
			valueType := prop.Value.GetComputedType()
			if valueType == nil {
				valueType = types.Any
			}
			fields[keyName] = valueType
		} else if _, isArrowFunction := prop.Value.(*parser.ArrowFunctionLiteral); isArrowFunction {
			// Arrow functions don't bind 'this', so we can visit them normally
			// But for consistency, let's still visit them in this pass
			debugPrintf("// [Checker ObjectLit] Visiting arrow function property '%s'\n", keyName)
			c.visit(prop.Value)
			valueType := prop.Value.GetComputedType()
			if valueType == nil {
				valueType = types.Any
			}
			fields[keyName] = valueType
		} else if _, isShorthandMethod := prop.Value.(*parser.ShorthandMethod); isShorthandMethod {
			// Shorthand methods bind 'this' like regular function methods
			debugPrintf("// [Checker ObjectLit] Visiting shorthand method '%s' with this context\n", keyName)
			c.visit(prop.Value)
			valueType := prop.Value.GetComputedType()
			if valueType == nil {
				valueType = types.Any
			}
			fields[keyName] = valueType
		} else if methodDef, isMethodDef := prop.Value.(*parser.MethodDefinition); isMethodDef {
			// Handle getter/setter methods
			debugPrintf("// [Checker ObjectLit] Visiting method definition '%s' (kind: %s) with this context\n", keyName, methodDef.Kind)
			c.visit(prop.Value)
			valueType := prop.Value.GetComputedType()
			if valueType == nil {
				valueType = types.Any
			}

			// Store getter/setter with appropriate prefix to match compiler expectations
			if methodDef.Kind == "getter" {
				// For getters, store both the implementation and the property type
				getterName := "__get__" + keyName
				fields[getterName] = valueType // Implementation for compiler

				// Also store the property with its return type for type checking
				if objType, ok := valueType.(*types.ObjectType); ok && objType.IsCallable() && len(objType.CallSignatures) > 0 {
					fields[keyName] = objType.CallSignatures[0].ReturnType // Property type for type checker
				} else {
					fields[keyName] = types.Any
				}
				debugPrintf("// [Checker ObjectLit] Stored getter as '%s' and property as '%s'\n", getterName, keyName)
			} else if methodDef.Kind == "setter" {
				// For setters, store both the implementation and make the property writable
				setterName := "__set__" + keyName
				fields[setterName] = valueType // Implementation for compiler

				// Also store the property with its parameter type for type checking
				if objType, ok := valueType.(*types.ObjectType); ok && objType.IsCallable() && len(objType.CallSignatures) > 0 && len(objType.CallSignatures[0].ParameterTypes) > 0 {
					fields[keyName] = objType.CallSignatures[0].ParameterTypes[0] // Property type for type checker
				} else {
					fields[keyName] = types.Any
				}
				debugPrintf("// [Checker ObjectLit] Stored setter as '%s' and property as '%s'\n", setterName, keyName)
			} else {
				// Regular method
				fields[keyName] = valueType
				debugPrintf("// [Checker ObjectLit] Stored method definition as '%s'\n", keyName)
			}
		}
	}

	// Restore outer this context
	c.currentThisType = outerThisType
	debugPrintf("// [Checker ObjectLit] Restored this context to: %v\n", outerThisType)

	// Handle computed properties by creating index signatures
	var hasComputedProperties bool
	var computedValueTypes []types.Type
	finalFields := make(map[string]types.Type)

	for key, valueType := range fields {
		if key == "__COMPUTED_PROPERTY__" {
			hasComputedProperties = true
			computedValueTypes = append(computedValueTypes, valueType)
			// Special-case: if the computed key expression was Symbol.iterator, record an explicit property marker
			// so iterable detection can succeed.
			finalFields["__COMPUTED_PROPERTY__"] = valueType
		} else {
			finalFields[key] = valueType
		}
	}

	// Create the final ObjectType
	objType := &types.ObjectType{Properties: finalFields}

	// If we have computed properties, add an index signature
	if hasComputedProperties {
		// Create union of all computed value types
		var indexValueType types.Type
		if len(computedValueTypes) == 1 {
			indexValueType = computedValueTypes[0]
		} else if len(computedValueTypes) > 1 {
			indexValueType = types.NewUnionType(computedValueTypes...)
		} else {
			indexValueType = types.Any
		}

		// Add string index signature
		objType.IndexSignatures = []*types.IndexSignature{
			{
				KeyType:   types.String,
				ValueType: indexValueType,
			},
		}
	}

	// Set the computed type for the ObjectLiteral node itself
	node.SetComputedType(objType)
	debugPrintf("// [Checker ObjectLit] Computed type: %s\n", objType.String())
	debugPrintf("// [Checker ObjectLit] Has computed properties: %v, final fields: %v\n", hasComputedProperties, finalFields)
}

// --- NEW: Template Literal Check ---
func (c *Checker) checkTemplateLiteral(node *parser.TemplateLiteral) {
	// Template literals always evaluate to string type, regardless of interpolated expressions
	// But we still need to visit all the parts to check for type errors

	for _, part := range node.Parts {
		switch p := part.(type) {
		case *parser.TemplateStringPart:
			// String parts don't need type checking - they're always strings
			// TemplateStringPart doesn't implement Expression interface, so no SetComputedType
			debugPrintf("// [Checker TemplateLit] Processing string part: '%s'\n", p.Value)

		default:
			// Expression parts: visit them to check for type errors
			c.visit(part)
			// Get the computed type (cast to Expression interface for safety)
			if expr, ok := part.(parser.Expression); ok {
				exprType := expr.GetComputedType()
				if exprType == nil {
					exprType = types.Any // Handle potential error
				}

				// In JavaScript/TypeScript, any expression in template literal interpolation
				// gets converted to string, so we don't need to enforce any particular type.
				// However, we can warn about problematic types if needed in the future.
				debugPrintf("// [Checker TemplateLit] Interpolated expression type: %s\n", exprType.String())
			} else {
				debugPrintf("// [Checker TemplateLit] WARNING: Non-expression part in template literal: %T\n", part)
			}
		}
	}

	// Template literals always result in string type
	node.SetComputedType(types.String)
	debugPrintf("// [Checker TemplateLit] Set template literal type to: string\n")
}

// checkTaggedTemplateExpression: the result type is any (string) for now; proper semantics later
func (c *Checker) checkTaggedTemplateExpression(node *parser.TaggedTemplateExpression) {
	// Check tag expression
	c.visit(node.Tag)
	// Check template parts
	c.checkTemplateLiteral(node.Template)
	// Result of a tag call is Any for now
	node.SetComputedType(types.Any)
}

// Helper function
func (c *Checker) checkMemberExpression(node *parser.MemberExpression) {
	// Check if there's a narrowed type for this member expression
	memberKey := expressionToNarrowingKey(node)
	if memberKey != "" {
		if narrowedType, exists := c.env.narrowings[memberKey]; exists {
			debugPrintf("// [MemberExpr] Using narrowed type for %s: %s\n", memberKey, narrowedType.String())
			node.SetComputedType(narrowedType)
			return
		}
		// Check for complement narrowing (from else branch)
		if complementType, exists := c.env.narrowings[memberKey+"__complement"]; exists {
			// Get the original type by visiting the expression
			c.visit(node.Object)
			objectType := node.Object.GetComputedType()
			if objectType != nil {
				propertyName := c.extractPropertyName(node.Property)
				if objType, ok := types.GetWidenedType(objectType).(*types.ObjectType); ok {
					if propType, found := objType.Properties[propertyName]; found {
						if unionType, ok := propType.(*types.UnionType); ok {
							// Compute complement: union minus the narrowed type
							remainingType := unionType.RemoveType(complementType)
							debugPrintf("// [MemberExpr] Using complement narrowing for %s: %s (removing %s)\n", memberKey, remainingType.String(), complementType.String())
							node.SetComputedType(remainingType)
							return
						}
					}
				}
			}
		}
		// Also check outer environments
		for env := c.env.outer; env != nil; env = env.outer {
			if env.narrowings != nil {
				if narrowedType, exists := env.narrowings[memberKey]; exists {
					debugPrintf("// [MemberExpr] Using narrowed type from outer env for %s: %s\n", memberKey, narrowedType.String())
					node.SetComputedType(narrowedType)
					return
				}
				// Check complement in outer envs too
				if complementType, exists := env.narrowings[memberKey+"__complement"]; exists {
					c.visit(node.Object)
					objectType := node.Object.GetComputedType()
					if objectType != nil {
						propertyName := c.extractPropertyName(node.Property)
						if objType, ok := types.GetWidenedType(objectType).(*types.ObjectType); ok {
							if propType, found := objType.Properties[propertyName]; found {
								if unionType, ok := propType.(*types.UnionType); ok {
									remainingType := unionType.RemoveType(complementType)
									debugPrintf("// [MemberExpr] Using complement narrowing from outer env for %s: %s\n", memberKey, remainingType.String())
									node.SetComputedType(remainingType)
									return
								}
							}
						}
					}
				}
			}
		}
	}

	// 1. Visit the object part
	c.visit(node.Object)
	objectType := node.Object.GetComputedType()
	if objectType == nil {
		// If visiting the object failed, checker should have added error.
		// Set objectType to Any to prevent cascading nil errors here.
		objectType = types.Any
	}

	// 2. Get the property name (Property can be Identifier or ComputedPropertyName)
	propertyName := c.extractPropertyName(node.Property)

	// 3. Widen the object type for checks
	widenedObjectType := types.GetWidenedType(objectType)

	var resultType types.Type = types.Never // Default to Never if property not found/invalid access

	// 4. Handle different base types
	if widenedObjectType == types.Any {
		resultType = types.Any // Property access on 'any' results in 'any'
	} else if widenedObjectType == types.String {
		if propertyName == "length" {
			resultType = types.Number // string.length is number
		} else {
			// Check prototype registry for String methods
			if methodType := c.env.GetPrimitivePrototypeMethodType("string", propertyName); methodType != nil {
				resultType = methodType
			} else {
				c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on type 'string'", propertyName))
				// resultType remains types.Never
			}
		}
	} else if widenedObjectType == types.Number {
		// Check prototype registry for Number methods
		if methodType := c.env.GetPrimitivePrototypeMethodType("number", propertyName); methodType != nil {
			resultType = methodType
		} else {
			c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on type 'number'", propertyName))
			// resultType remains types.Never
		}
	} else if widenedObjectType == types.RegExp {
		// Check prototype registry for RegExp methods and properties
		if methodType := c.env.GetPrimitivePrototypeMethodType("RegExp", propertyName); methodType != nil {
			resultType = methodType
		} else {
			c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on type 'RegExp'", propertyName))
			// resultType remains types.Never
		}
	} else if widenedObjectType == types.Symbol {
		// Check prototype registry for Symbol methods and properties
		if methodType := c.env.GetPrimitivePrototypeMethodType("symbol", propertyName); methodType != nil {
			resultType = methodType
		} else {
			c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on type 'symbol'", propertyName))
			// resultType remains types.Never
		}
	} else {
		// Use a type switch for struct-based types
		switch obj := widenedObjectType.(type) {
		case *types.ArrayType:
			if propertyName == "length" {
				resultType = types.Number // Array.length is number
			} else {
				// Check prototype registry for Array methods
				if methodType := c.env.GetPrimitivePrototypeMethodType("array", propertyName); methodType != nil {
					// If the method is generic, instantiate it with the array's element type
					resultType = c.instantiateGenericMethod(methodType, obj.ElementType)
				} else {
					c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on type %s", propertyName, obj.String()))
					// resultType remains types.Never
				}
			}
		case *types.ObjectType: // <<< MODIFIED CASE
			// Check if this is a function and we're accessing 'prototype'
			if propertyName == "prototype" && obj != nil && obj.IsCallable() {
				// Function.prototype returns 'any' in TypeScript to allow dynamic assignment
				resultType = types.Any
				debugPrintf("// [Checker MemberExpr] Function.prototype access, returning 'any' type\n")
			} else {
				// Look for the property in the object's fields, including inherited properties
				effectiveProps := obj.GetEffectiveProperties()
				fieldType, exists := effectiveProps[propertyName]
				if exists {
					// Property found - check access control for class types
					c.validateMemberAccess(objectType, propertyName, node.Property)

					if fieldType == nil { // Should ideally not happen if checker populates correctly
						c.addError(node.Property, fmt.Sprintf("internal checker error: property '%s' has nil type in ObjectType", propertyName))
						resultType = types.Never
					} else {
						resultType = fieldType
					}
				} else {
					// Property not found in explicit properties - check index signatures
					if len(obj.IndexSignatures) > 0 {
						debugPrintf("// [Checker MemberExpr] Property '%s' not found, checking %d index signatures\n", propertyName, len(obj.IndexSignatures))
						for _, indexSig := range obj.IndexSignatures {
							// For string index signatures, allow any string property access
							if indexSig.KeyType == types.String {
								resultType = indexSig.ValueType
								debugPrintf("// [Checker MemberExpr] Property '%s' matches string index signature: %s\n", propertyName, resultType.String())
								break
							}
							// TODO: Handle number index signatures, symbol index signatures, etc.
						}
					}

					if resultType == types.Never {
						if obj.IsCallable() {
							// Check for function prototype methods if this is a callable object
							if methodType := c.env.GetPrimitivePrototypeMethodType("function", propertyName); methodType != nil {
								resultType = methodType
								debugPrintf("// [Checker MemberExpr] Found function prototype method '%s': %s\n", propertyName, methodType.String())
							} else {
								// NEW: Check for Object prototype methods for all objects
								if methodType := c.env.GetPrimitivePrototypeMethodType("object", propertyName); methodType != nil {
									resultType = methodType
									debugPrintf("// [Checker MemberExpr] Found object prototype method '%s': %s\n", propertyName, methodType.String())
								} else {
									// Property not found
									c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on type %s", propertyName, obj.String()))
									// resultType remains types.Never
								}
							}
						} else {
							// NEW: Check for Object prototype methods for all objects
							if methodType := c.env.GetPrimitivePrototypeMethodType("object", propertyName); methodType != nil {
								resultType = methodType
								debugPrintf("// [Checker MemberExpr] Found object prototype method '%s': %s\n", propertyName, methodType.String())
							} else {
								// Property not found
								c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on type %s", propertyName, obj.String()))
								// resultType remains types.Never
							}
						}
					}
				}
			}
		case *types.IntersectionType:
			// Handle property access on intersection types
			propType := c.getPropertyTypeFromIntersection(obj, propertyName)
			if propType == types.Never {
				c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on intersection type %s", propertyName, obj.String()))
			}
			resultType = propType
		case *types.ReadonlyType:
			// Handle property access on readonly types
			// Delegate to the inner type for property lookup
			innerType := obj.InnerType

			// Check property access on the inner type
			if innerType == types.Any {
				// For Readonly<any>, allow any property access
				resultType = types.Any
			} else {
				switch innerObj := innerType.(type) {
				case *types.ObjectType:
					if propType, exists := innerObj.Properties[propertyName]; exists {
						// Property exists, return its type (not wrapped in readonly for reading)
						resultType = propType
					} else {
						// Check if property is optional
						isOptional := innerObj.OptionalProperties != nil && innerObj.OptionalProperties[propertyName]
						if !isOptional {
							c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on readonly type", propertyName))
							resultType = types.Never
						} else {
							resultType = types.Undefined // Optional property that doesn't exist
						}
					}
				default:
					// For non-object inner types, we can't access properties
					c.addError(node.Object, fmt.Sprintf("property access is not supported on readonly %s", innerType.String()))
					resultType = types.Never
				}
			}
		case *types.MappedType:
			// Handle property access on mapped types by expanding them first
			debugPrintf("// [Checker MemberExpr] Found mapped type, expanding for property access: %s\n", obj.String())
			expandedType := c.expandIfMappedType(obj)
			if expandedObj, ok := expandedType.(*types.ObjectType); ok {
				debugPrintf("// [Checker MemberExpr] Mapped type expanded to ObjectType: %s\n", expandedObj.String())
				if propType, exists := expandedObj.Properties[propertyName]; exists {
					resultType = propType
					debugPrintf("// [Checker MemberExpr] Found property '%s' in expanded type: %s\n", propertyName, propType.String())
				} else {
					// Check if property is optional
					isOptional := expandedObj.OptionalProperties != nil && expandedObj.OptionalProperties[propertyName]
					if !isOptional {
						c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on type %s", propertyName, obj.String()))
						resultType = types.Never
					} else {
						resultType = types.Undefined
					}
				}
			} else if expandedType == types.Any {
				// Mapped type expanded to any (e.g., Readonly<any>)
				debugPrintf("// [Checker MemberExpr] Mapped type expanded to any, allowing property access\n")
				resultType = types.Any
			} else {
				debugPrintf("// [Checker MemberExpr] Mapped type expansion failed, result: %T %s\n", expandedType, expandedType.String())
				c.addError(node.Object, fmt.Sprintf("property access is not supported on mapped type %s", obj.String()))
				resultType = types.Never
			}
		case *types.InstantiatedType:
			// Handle property access on instantiated generic types (like Map<K,V>)
			substitutedType := obj.Substitute()
			if substitutedType != nil {
				// Create a temporary member expression to re-check with the substituted type
				// but avoid infinite recursion by using the substituted type directly
				switch subst := substitutedType.(type) {
				case *types.ObjectType:
					// Look for the property in the substituted object's fields
					fieldType, exists := subst.Properties[propertyName]
					if exists {
						resultType = fieldType
					} else {
						c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on type %s", obj.String(), subst.String()))
						// resultType remains types.Never
					}
				default:
					// For other substituted types, recursively check member access
					// We need to temporarily change the object type and re-check
					savedObjectType := node.Object.GetComputedType()
					node.Object.SetComputedType(substitutedType)
					c.checkMemberExpression(node)
					resultType = node.GetComputedType()
					node.Object.SetComputedType(savedObjectType)
					return // Skip setting the computed type again at the end
				}
			} else {
				c.addError(node.Object, fmt.Sprintf("failed to substitute generic type %s", obj.String()))
				// resultType remains types.Never
			}
		case *types.TypeParameterType:
			// Handle property access on type parameters
			if obj.Parameter != nil && obj.Parameter.Constraint != nil {
				// If the type parameter has a constraint, check property access on the constraint
				constraintType := obj.Parameter.Constraint
				debugPrintf("// [Checker MemberExpr] Type parameter '%s' has constraint: %s, checking property '%s'\n",
					obj.Parameter.Name, constraintType.String(), propertyName)

				// Use the helper function to get property type from the constraint
				resultType = c.getPropertyTypeFromType(constraintType, propertyName, false)
			} else {
				// For unconstrained type parameters, allow property access but return 'any'
				// This is because the type parameter could be instantiated with any type that has this property
				resultType = types.Any
				debugPrintf("// [Checker MemberExpr] Unconstrained type parameter, allowing property access: %s\n", propertyName)
			}
		case *types.UnionType:
			// For union types, check if the property exists on all members
			var possibleTypes []types.Type
			allMembersHaveProperty := true

			for _, memberType := range obj.Types {
				// Create a temporary member expression to check this member type
				memberHasProperty := false
				var memberResultType types.Type

				// Check what type this member would produce for the property
				switch member := memberType.(type) {
				case *types.ObjectType:
					if fieldType, exists := member.Properties[propertyName]; exists {
						memberHasProperty = true
						memberResultType = fieldType
					}
				case *types.ArrayType:
					if propertyName == "length" {
						memberHasProperty = true
						memberResultType = types.Number
					}
					// Add other cases as needed for primitives with prototypes, etc.
				}

				if memberHasProperty {
					possibleTypes = append(possibleTypes, memberResultType)
				} else {
					allMembersHaveProperty = false
					break
				}
			}

			if allMembersHaveProperty && len(possibleTypes) > 0 {
				// All members have the property, create union of result types
				if len(possibleTypes) == 1 {
					resultType = possibleTypes[0]
				} else {
					resultType = types.NewUnionType(possibleTypes...)
				}
			} else {
				c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on all members of union type %s", propertyName, obj.String()))
				resultType = types.Never
			}
		case *types.EnumType:
			// Handle property access on enum types (enum member access)
			if memberType, exists := obj.Members[propertyName]; exists {
				resultType = memberType
				debugPrintf("// [Checker MemberExpr] Found enum member '%s.%s': %s\n", obj.Name, propertyName, memberType.String())
			} else {
				c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on enum %s", propertyName, obj.Name))
				resultType = types.Never
			}
		case *types.ForwardReferenceType:
			// Handle static member access on forward reference (class name used within class)
			resultType = c.handleForwardReferenceStaticAccess(obj, propertyName, node)
		case *types.ParameterizedForwardReferenceType:
			// Handle property access on parameterized generic types like LinkedNode<T>
			debugPrintf("// [Checker MemberExpr] ParameterizedForwardReferenceType: %s, property: %s\n", obj.String(), propertyName)

			// The issue is that during method body checking, the generic class might not be fully resolved yet
			// Instead of trying to resolve the constructor, resolve the ParameterizedForwardReferenceType directly
			resolvedType := c.resolveParameterizedForwardReference(obj)
			if resolvedType != nil {
				debugPrintf("// [Checker MemberExpr] Resolved ParameterizedForwardReferenceType to: %T = %s\n", resolvedType, resolvedType.String())
				if objType, ok := resolvedType.(*types.ObjectType); ok {
					// Check if the property exists on the resolved type
					if propType, exists := objType.Properties[propertyName]; exists {
						resultType = propType
						debugPrintf("// [Checker MemberExpr] Found property '%s' on resolved type: %s\n", propertyName, propType.String())
					} else {
						c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on type %s", propertyName, resolvedType.String()))
						resultType = types.Never
					}
				} else {
					c.addError(node.Object, fmt.Sprintf("resolved type %s is not an object type", resolvedType.String()))
					resultType = types.Never
				}
			} else {
				c.addError(node.Object, fmt.Sprintf("could not resolve parameterized forward reference %s", obj.String()))
				resultType = types.Never
			}
		// Add cases for other struct-based types here if needed
		default:
			// This covers cases where widenedObjectType was not String, Any, ArrayType, ObjectType, etc.
			// e.g., trying to access property on number, boolean, null, undefined
			c.addError(node.Object, fmt.Sprintf("property access is not supported on type %s", widenedObjectType.String()))
			// resultType remains types.Error
		}
	}

	// 5. Set the computed type on the MemberExpression node itself
	node.SetComputedType(resultType)
	debugPrintf("// [Checker MemberExpr] ObjectType: %s, Property: %s, ResultType: %s\n", objectType.String(), propertyName, resultType.String())
}

// handleForwardReferenceStaticAccess handles static member access on a forward reference
func (c *Checker) handleForwardReferenceStaticAccess(forwardRef *types.ForwardReferenceType, propertyName string, node *parser.MemberExpression) types.Type {
	// For forward references, we need to check if we're accessing static members
	// of the class currently being defined. Use the current class context.

	// First, try to look up the resolved constructor type
	constructorType, _, exists := c.env.Resolve(forwardRef.ClassName)
	if exists {
		// Check if it's an ObjectType with static members
		if objType, ok := constructorType.(*types.ObjectType); ok {
			// Look for static property in the constructor object
			if propType, exists := objType.Properties[propertyName]; exists {
				return propType
			}

			// Check if it's a call signature (static method)
			for _, callSig := range objType.CallSignatures {
				if callSig.ParameterTypes != nil {
					// This is a static method call - return the method type
					return types.NewFunctionType(callSig)
				}
			}
		}

		// Handle generic class constructors
		if genericType, ok := constructorType.(*types.GenericType); ok {
			if constructorObj, ok := genericType.Body.(*types.ObjectType); ok {
				if propType, exists := constructorObj.Properties[propertyName]; exists {
					return propType
				}
			}
		}
	}

	// If the constructor type is not yet resolved, this is likely a self-reference
	// within the class being defined. For now, allow access to any static member
	// and let the compiler/runtime handle the validation.
	return types.Any // Allow static access during class definition
}

func (c *Checker) checkIndexExpression(node *parser.IndexExpression) {
	// 1. Visit the base expression (array/object)
	c.visit(node.Left)
	leftType := node.Left.GetComputedType()

	// 2. Visit the index expression
	c.visit(node.Index)
	indexType := node.Index.GetComputedType()

	var resultType types.Type = types.Any // Default result type on error

	// Defensive check: if leftType is nil, default to Any
	if leftType == nil {
		debugPrintf("// [Checker IndexExpr] Warning: left expression has nil type, defaulting to Any\n")
		leftType = types.Any
	}

	// Defensive check: if indexType is nil, default to Any
	if indexType == nil {
		debugPrintf("// [Checker IndexExpr] Warning: index expression has nil type, defaulting to Any\n")
		indexType = types.Any
	}

	// 3. Check base type (allow Array for now)
	// First handle the special case of 'any'
	if leftType == types.Any {
		// Indexing into 'any' always returns 'any' - this is standard TypeScript behavior
		resultType = types.Any
	} else {
		switch base := leftType.(type) {
		case *types.ArrayType:
			// Base is ArrayType
			// 4. Check index type (number for array elements, string/symbol for properties)
			if types.IsAssignable(indexType, types.Number) {
				// Numeric index - accessing array elements
				if base.ElementType != nil {
					resultType = base.ElementType
				} else {
					resultType = types.Unknown
				}
			} else if types.IsAssignable(indexType, types.String) || types.IsAssignable(indexType, types.Symbol) {
				// String or Symbol index - accessing array properties (like Symbol.iterator)
				resultType = types.Any // Arrays can have arbitrary properties
			} else {
				c.addError(node.Index, fmt.Sprintf("array index must be of type number, string, or symbol, got %s", indexType.String()))
				resultType = types.Any
			}

		case *types.ObjectType: // <<< NEW CASE
			// Base is ObjectType
			// Index must be string or number (or any)
			isIndexStringLiteral := false
			var indexStringValue string
			if litIndex, ok := indexType.(*types.LiteralType); ok && litIndex.Value.Type() == vm.TypeString {
				isIndexStringLiteral = true
				indexStringValue = vm.AsString(litIndex.Value)
			}

			widenedIndexType := types.GetWidenedType(indexType)

			if widenedIndexType == types.String || widenedIndexType == types.Number || widenedIndexType == types.Symbol || widenedIndexType == types.Any {
				if isIndexStringLiteral {
					// Index is a specific string literal, look it up directly
					propType, exists := base.Properties[indexStringValue]
					if exists {
						if propType == nil { // Safety check
							resultType = types.Never
							c.addError(node.Index, fmt.Sprintf("internal checker error: property '%s' has nil type", indexStringValue))
						} else {
							resultType = propType
						}
					} else {
						// In TypeScript, obj["unknownProp"] should return 'any' if no index signature exists
						// This is different from obj.unknownProp which should error
						// TODO: Check for index signatures first, for now default to 'any'
						resultType = types.Any
					}
				} else {
					// Index is a general string/number/any - cannot determine specific property type.
					// TODO: Support index signatures later?
					// For now, result is 'any' as we don't know which property is accessed.
					resultType = types.Any
				}
			} else {
				// Invalid index type for object
				c.addError(node.Index, fmt.Sprintf("object index must be of type 'string', 'number', 'symbol', or 'any', got '%s'", indexType.String()))
				// resultType remains Error
			}

		case *types.UnionType:
			// Handle index access on union types
			// For each member of the union that supports indexing, collect the result types
			var possibleTypes []types.Type
			allMembersSupported := true

			for _, memberType := range base.Types {
				switch member := memberType.(type) {
				case *types.ObjectType:
					// Check if this member supports the index type
					widenedIndexType := types.GetWidenedType(indexType)
					if widenedIndexType == types.String || widenedIndexType == types.Number || widenedIndexType == types.Symbol || widenedIndexType == types.Any {
						// For union types with object members, we can't determine the specific property
						// Return 'any' for the property access (conservative approach)
						possibleTypes = append(possibleTypes, types.Any)
					} else {
						allMembersSupported = false
						break
					}
				case *types.ArrayType:
					// Check if index is number for array member
					if types.IsAssignable(indexType, types.Number) {
						if member.ElementType != nil {
							possibleTypes = append(possibleTypes, member.ElementType)
						} else {
							possibleTypes = append(possibleTypes, types.Unknown)
						}
					} else {
						allMembersSupported = false
						break
					}
				case *types.Primitive:
					if member == types.String {
						if types.IsAssignable(indexType, types.Number) {
							// Numeric index - string character access
							possibleTypes = append(possibleTypes, types.String)
						} else if types.IsAssignable(indexType, types.String) || types.IsAssignable(indexType, types.Symbol) {
							// String/Symbol index - string property access
							possibleTypes = append(possibleTypes, types.Any)
						} else {
							allMembersSupported = false
							break
						}
					} else {
						allMembersSupported = false
						break
					}
				default:
					// This member doesn't support indexing
					allMembersSupported = false
					break
				}
			}

			if allMembersSupported && len(possibleTypes) > 0 {
				// All members support indexing, create union of result types
				resultType = types.NewUnionType(possibleTypes...)
			} else {
				// Some members don't support indexing
				c.addError(node.Left, fmt.Sprintf("cannot apply index operator to type %s", leftType.String()))
			}

		case *types.Primitive:
			// Allow indexing on strings?
			if base == types.String {
				// 4. Check index type (number for string characters, string/symbol for properties)
				if types.IsAssignable(indexType, types.Number) {
					// Numeric index - accessing string characters
					resultType = types.String
				} else if types.IsAssignable(indexType, types.String) || types.IsAssignable(indexType, types.Symbol) {
					// String or Symbol index - accessing string properties (like Symbol.iterator)
					resultType = types.Any // String properties can have arbitrary types
				} else {
					c.addError(node.Index, fmt.Sprintf("string index must be of type number, string, or symbol, got %s", indexType.String()))
					resultType = types.Any
				}
			} else {
				c.addError(node.Index, fmt.Sprintf("cannot apply index operator to type %s", leftType.String()))
			}

		case *types.EnumType:
			// Handle index access on enum types (reverse mapping for numeric enums)
			// Check if this is a numeric index access
			if types.IsAssignable(indexType, types.Number) {
				// Allow reverse mapping for numeric indices (works for both numeric and heterogeneous enums)
				resultType = types.String // Reverse mapping returns string member name
				debugPrintf("// [Checker IndexExpr] Enum reverse mapping: %s[number] -> string\n", base.Name)
			} else if types.IsAssignable(indexType, types.String) {
				// String index access - this is allowed but typically returns undefined for reverse mapping
				// For type checking purposes, we'll allow it as it could return string | undefined
				resultType = types.Any // In practice, this would be string | undefined
				debugPrintf("// [Checker IndexExpr] String index access on enum %s[string] -> any\n", base.Name)
			} else {
				// For other index types, this is an error
				c.addError(node.Index, fmt.Sprintf("enum %s can only be indexed with number or string, got %s", base.Name, indexType.String()))
			}

		case *types.InstantiatedType:
			// Handle instantiated generic types (like Generator<T, TReturn, TNext>)
			if base.Generic != nil && base.Generic.Body != nil {
				if baseObjectType, ok := base.Generic.Body.(*types.ObjectType); ok {
					// Check if this is a Generator type by looking for the next() method
					if _, hasNext := baseObjectType.Properties["next"]; hasNext {
						// This is likely a Generator or Iterator - allow Symbol indexing for Symbol.iterator
						if types.IsAssignable(indexType, types.Symbol) {
							resultType = types.Any // Symbol.iterator returns the iterator itself
						} else if types.IsAssignable(indexType, types.String) {
							resultType = types.Any // String properties allowed
						} else {
							c.addError(node.Index, fmt.Sprintf("generator/iterator index must be of type string or symbol, got %s", indexType.String()))
							resultType = types.Any
						}
					} else {
						// Generic object type - use general object indexing rules
						widenedIndexType := types.GetWidenedType(indexType)
						if widenedIndexType == types.String || widenedIndexType == types.Number || widenedIndexType == types.Symbol || widenedIndexType == types.Any {
							resultType = types.Any
						} else {
							c.addError(node.Index, fmt.Sprintf("object index must be of type 'string', 'number', 'symbol', or 'any', got '%s'", indexType.String()))
							resultType = types.Any
						}
					}
				} else {
					c.addError(node.Index, fmt.Sprintf("cannot apply index operator to type %s", leftType.String()))
				}
			} else {
				c.addError(node.Index, fmt.Sprintf("cannot apply index operator to type %s", leftType.String()))
			}

		default:
			c.addError(node.Index, fmt.Sprintf("cannot apply index operator to type %s", leftType.String()))
		}
	}

	// Set computed type on the IndexExpression node
	node.SetComputedType(resultType)
	debugPrintf("// [Checker IndexExpr] Computed type: %s\n", resultType.String())
}

// checkOptionalChainingExpression handles optional chaining property access (e.g., obj?.prop)
func (c *Checker) checkOptionalChainingExpression(node *parser.OptionalChainingExpression) {
	// 1. Visit the object part
	c.visit(node.Object)
	objectType := node.Object.GetComputedType()
	if objectType == nil {
		// If visiting the object failed, checker should have added error.
		// Set objectType to Any to prevent cascading nil errors here.
		objectType = types.Any
	}

	// 2. Get the property name (Property can be Identifier or ComputedPropertyName)
	propertyName := c.extractPropertyName(node.Property)

	// 3. Widen the object type for checks
	widenedObjectType := types.GetWidenedType(objectType)

	var baseResultType types.Type = types.Never // Default to Never if property not found/invalid access

	// 4. Handle different base types (similar to MemberExpression but more permissive)
	if widenedObjectType == types.Any {
		baseResultType = types.Any // Property access on 'any' results in 'any'
	} else if widenedObjectType == types.Null || widenedObjectType == types.Undefined {
		// Optional chaining on null/undefined is safe and returns undefined
		baseResultType = types.Undefined
	} else if widenedObjectType == types.String {
		if propertyName == "length" {
			baseResultType = types.Number // string.length is number
		} else {
			// Check prototype registry for String methods
			if methodType := c.env.GetPrimitivePrototypeMethodType("string", propertyName); methodType != nil {
				baseResultType = methodType
			} else {
				// Property not found - for optional chaining, this is OK, just return undefined
				baseResultType = types.Undefined
			}
		}
	} else {
		// Use a type switch for struct-based types
		switch obj := widenedObjectType.(type) {
		case *types.UnionType:
			// Handle union types - for optional chaining, we need to handle each type in the union
			var nonNullUndefinedTypes []types.Type

			// Separate null/undefined types from others
			for _, t := range obj.Types {
				if t == types.Null || t == types.Undefined {
					// Skip null/undefined types - they are handled by optional chaining
				} else {
					nonNullUndefinedTypes = append(nonNullUndefinedTypes, t)
				}
			}

			if len(nonNullUndefinedTypes) == 0 {
				// Union contains only null/undefined types
				baseResultType = types.Undefined
			} else if len(nonNullUndefinedTypes) == 1 {
				// Union has one non-null/undefined type - use that for property access
				nonNullType := nonNullUndefinedTypes[0]
				baseResultType = c.getPropertyTypeFromType(nonNullType, propertyName, true) // true for optional chaining
			} else {
				// Union has multiple non-null/undefined types - this is complex
				// For now, try to access the property on each type and create a union of results
				var resultTypes []types.Type
				for _, t := range nonNullUndefinedTypes {
					propType := c.getPropertyTypeFromType(t, propertyName, true)
					if propType != types.Never {
						resultTypes = append(resultTypes, propType)
					}
				}
				if len(resultTypes) == 0 {
					baseResultType = types.Undefined
				} else {
					baseResultType = types.NewUnionType(resultTypes...)
				}
			}
		case *types.ArrayType:
			if propertyName == "length" {
				baseResultType = types.Number // Array.length is number
			} else {
				// Array methods should be resolved through the builtins system
				c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on type %s", propertyName, obj.String()))
				// baseResultType remains types.Never
			}
		case *types.ObjectType:
			// Look for the property in the object's fields
			fieldType, exists := obj.Properties[propertyName]
			if exists {
				// Property found
				if fieldType == nil { // Should ideally not happen if checker populates correctly
					c.addError(node.Property, fmt.Sprintf("internal checker error: property '%s' has nil type in ObjectType", propertyName))
					baseResultType = types.Never
				} else {
					baseResultType = fieldType
				}
			} else if obj.IsCallable() {
				// Check for function prototype methods if this is a callable object
				if methodType := c.env.GetPrimitivePrototypeMethodType("function", propertyName); methodType != nil {
					baseResultType = methodType
					debugPrintf("// [Checker OptionalChaining] Found function prototype method '%s': %s\n", propertyName, methodType.String())
				} else {
					// Property not found - for optional chaining, this is OK, just return undefined
					// Don't add an error like regular member access would
					baseResultType = types.Undefined
				}
			} else {
				// Property not found - for optional chaining, this is OK, just return undefined
				// Don't add an error like regular member access would
				baseResultType = types.Undefined
			}
		case *types.TypeParameterType:
			// Handle property access on type parameters
			if obj.Parameter != nil && obj.Parameter.Constraint != nil {
				// If the type parameter has a constraint, check property access on the constraint
				constraintType := obj.Parameter.Constraint
				baseResultType = c.getPropertyTypeFromType(constraintType, propertyName, true)
			} else {
				// For unconstrained type parameters, allow property access but return 'any'
				// This is because the type parameter could be instantiated with any type that has this property
				baseResultType = types.Any
			}
		// Add cases for other struct-based types here if needed (e.g., FunctionType methods?)
		default:
			// This covers cases where widenedObjectType was not String, Any, ArrayType, ObjectType, etc.
			// e.g., trying to access property on number, boolean, null, undefined
			c.addError(node.Object, fmt.Sprintf("property access is not supported on type %s", widenedObjectType.String()))
			// baseResultType remains types.Never
		}
	}

	// 5. For optional chaining, the result type is always a union with undefined
	// unless the object is already null/undefined (in which case it's just undefined)
	var resultType types.Type
	if widenedObjectType == types.Null || widenedObjectType == types.Undefined {
		resultType = types.Undefined
	} else if baseResultType == types.Never {
		// If property access failed, optional chaining still returns undefined instead of error
		resultType = types.Undefined
	} else {
		// Create union type: baseResultType | undefined
		resultType = &types.UnionType{
			Types: []types.Type{baseResultType, types.Undefined},
		}
	}

	// 6. Set the computed type on the OptionalChainingExpression node itself
	node.SetComputedType(resultType)
	debugPrintf("// [Checker OptionalChaining] ObjectType: %s, Property: %s, ResultType: %s\n", objectType.String(), propertyName, resultType.String())
}

// checkOptionalIndexExpression handles optional computed property access (e.g., obj?.[expr])
func (c *Checker) checkOptionalIndexExpression(node *parser.OptionalIndexExpression) {
	// 1. Visit the object part
	c.visit(node.Object)
	objectType := node.Object.GetComputedType()
	if objectType == nil {
		objectType = types.Any
	}

	// 2. Visit the index expression
	c.visit(node.Index)
	indexType := node.Index.GetComputedType()
	if indexType == nil {
		indexType = types.Any
	}

	// 3. Check that index type is valid (string, number, or symbol)
	widenedIndexType := types.GetWidenedType(indexType)
	if widenedIndexType != types.Any && widenedIndexType != types.String && widenedIndexType != types.Number && widenedIndexType != types.Symbol {
		c.addError(node.Index, "Index signature parameter type must be 'string', 'number', 'symbol' or a template literal pattern")
	}

	// 4. Determine result type (similar to IndexExpression but with optional chaining)
	widenedObjectType := types.GetWidenedType(objectType)
	var baseResultType types.Type = types.Any

	if widenedObjectType == types.Any {
		baseResultType = types.Any
	} else if widenedObjectType == types.Null || widenedObjectType == types.Undefined {
		baseResultType = types.Undefined
	} else if arrayType, ok := widenedObjectType.(*types.ArrayType); ok {
		// Array access - check if index is numeric
		if widenedIndexType == types.Number || widenedIndexType == types.Any {
			baseResultType = arrayType.ElementType
		} else {
			baseResultType = types.Undefined
		}
	} else {
		// Object access - for optional chaining, be permissive
		baseResultType = types.Any
	}

	// 5. For optional chaining, always union with undefined
	var resultType types.Type
	if baseResultType == types.Undefined {
		resultType = types.Undefined
	} else {
		resultType = &types.UnionType{
			Types: []types.Type{baseResultType, types.Undefined},
		}
	}

	node.SetComputedType(resultType)
	debugPrintf("// [Checker OptionalIndex] ObjectType: %s, IndexType: %s, ResultType: %s\n", objectType.String(), indexType.String(), resultType.String())
}

// checkOptionalCallExpression handles optional function calls (e.g., func?.())
func (c *Checker) checkOptionalCallExpression(node *parser.OptionalCallExpression) {
	// 1. Visit the function part
	c.visit(node.Function)
	functionType := node.Function.GetComputedType()
	if functionType == nil {
		functionType = types.Any
	}

	// 2. Visit arguments
	var argumentTypes []types.Type
	for _, arg := range node.Arguments {
		c.visit(arg)
		argType := arg.GetComputedType()
		if argType == nil {
			argType = types.Any
		}
		argumentTypes = append(argumentTypes, argType)
	}

	// 3. Determine result type
	widenedFunctionType := types.GetWidenedType(functionType)
	var baseResultType types.Type = types.Any

	if widenedFunctionType == types.Any {
		baseResultType = types.Any
	} else if widenedFunctionType == types.Null || widenedFunctionType == types.Undefined {
		baseResultType = types.Undefined
	} else {
		// For optional call, we're more permissive - just try to get return type
		if objType, ok := widenedFunctionType.(*types.ObjectType); ok {
			// Check if it has a call signature
			if len(objType.CallSignatures) > 0 {
				baseResultType = objType.CallSignatures[0].ReturnType
			} else {
				baseResultType = types.Any
			}
		} else {
			// Assume it might be callable and return Any
			baseResultType = types.Any
		}
	}

	// 4. For optional chaining, always union with undefined
	var resultType types.Type
	if baseResultType == types.Undefined {
		resultType = types.Undefined
	} else {
		resultType = &types.UnionType{
			Types: []types.Type{baseResultType, types.Undefined},
		}
	}

	node.SetComputedType(resultType)
	debugPrintf("// [Checker OptionalCall] FunctionType: %s, ResultType: %s\n", functionType.String(), resultType.String())
}

func (c *Checker) checkNewExpression(node *parser.NewExpression) {
	debugPrintf("// [Checker NewExpression] Checking new expression with %d type arguments\n", len(node.TypeArguments))

	// Check the constructor expression
	c.visit(node.Constructor)
	constructorType := node.Constructor.GetComputedType()
	if constructorType == nil {
		constructorType = types.Any
	}
	debugPrintf("// [Checker NewExpression] Constructor type: %T = %s\n", constructorType, constructorType.String())

	// Handle generic constructor calls (e.g., new Container<number>(42))
	if len(node.TypeArguments) > 0 {
		debugPrintf("// [Checker NewExpression] Processing %d type arguments\n", len(node.TypeArguments))
		// Check type arguments - these are type annotations, not expressions
		typeArgs := make([]types.Type, len(node.TypeArguments))
		for i, arg := range node.TypeArguments {
			// Don't call c.visit(arg) here - type arguments are not expressions
			typeArgs[i] = c.resolveTypeAnnotation(arg)
			if typeArgs[i] == nil {
				typeArgs[i] = types.Any
			}
			debugPrintf("// [Checker NewExpression] Type arg %d: %s\n", i, typeArgs[i].String())
		}

		// If constructor is a generic type, instantiate it
		if genericType, ok := constructorType.(*types.GenericType); ok {
			debugPrintf("// [Checker NewExpression] Instantiating generic constructor '%s' with %d type args\n",
				genericType.Name, len(typeArgs))
			instantiatedConstructorType := c.instantiateGenericType(genericType, typeArgs, node.TypeArguments)
			debugPrintf("// [Checker NewExpression] Instantiated constructor type: %T = %s\n",
				instantiatedConstructorType, instantiatedConstructorType.String())
			constructorType = instantiatedConstructorType
		}
	}

	// Check if trying to instantiate an abstract class
	if ident, ok := node.Constructor.(*parser.Identifier); ok {
		if c.abstractClasses[ident.Value] {
			c.addError(node, fmt.Sprintf("cannot create an instance of an abstract class '%s'", ident.Value))
			node.SetComputedType(types.Any)
			return
		}
	}

	// Check if constructor is a generic type without explicit type arguments
	if genericType, ok := constructorType.(*types.GenericType); ok && len(node.TypeArguments) == 0 {
		debugPrintf("// [Checker NewExpression] Generic constructor without type arguments, attempting inference\n")

		// Check arguments to get their types
		var argTypes []types.Type
		for _, arg := range node.Arguments {
			c.visit(arg)
			argType := arg.GetComputedType()
			if argType == nil {
				argType = types.Any
			}
			argTypes = append(argTypes, argType)
		}

		// Get the constructor signature from the generic type's body
		if bodyObjType, ok := genericType.Body.(*types.ObjectType); ok && len(bodyObjType.ConstructSignatures) > 0 {
			constructorSig := bodyObjType.ConstructSignatures[0]

			// Check if this is a generic signature
			if c.isGenericSignature(constructorSig) {
				debugPrintf("// [Checker NewExpression] Attempting type inference for generic constructor\n")

				// Create a pseudo CallExpression for the inference function
				pseudoCall := &parser.CallExpression{
					Arguments: node.Arguments,
				}

				// Infer type parameters
				inferredSig := c.inferGenericFunctionCall(pseudoCall, constructorSig)
				if inferredSig != nil {
					debugPrintf("// [Checker NewExpression] Type inference successful\n")

					// Extract inferred type parameters from the signature
					// For now, we'll use a simple approach: collect all TypeParameterType replacements
					typeArgs := c.extractInferredTypeArguments(genericType, constructorSig, inferredSig)

					if len(typeArgs) > 0 {
						// Instantiate the generic type with inferred arguments
						instantiatedConstructorType := c.instantiateGenericType(genericType, typeArgs, nil)
						constructorType = instantiatedConstructorType
						debugPrintf("// [Checker NewExpression] Instantiated constructor with inferred types: %s\n", constructorType.String())
					}
				} else {
					debugPrintf("// [Checker NewExpression] Type inference failed\n")
				}
			}
		}
	} else {
		// Check arguments with proper validation
		// First, try to get the constructor signature to validate arguments
		var constructorSig *types.Signature
		if objType, ok := constructorType.(*types.ObjectType); ok && len(objType.ConstructSignatures) > 0 {
			// For now, only validate if there's a single constructor signature
			// TODO: Implement overload resolution for constructors
			if len(objType.ConstructSignatures) == 1 {
				constructorSig = objType.ConstructSignatures[0]
			}
		}

		if constructorSig != nil {
			debugPrintf("// [Checker NewExpression] Got constructor signature: IsVariadic=%v, RestParamType=%v, Params=%d\n",
				constructorSig.IsVariadic, constructorSig.RestParameterType, len(constructorSig.ParameterTypes))

			// Validate spread arguments first
			hasSpreadErrors := false
			currentArgIndex := 0
			for _, arg := range node.Arguments {
				if spreadElement, isSpread := arg.(*parser.SpreadElement); isSpread {
					if !c.validateSpreadArgument(spreadElement, constructorSig.IsVariadic, constructorSig.ParameterTypes, currentArgIndex) {
						hasSpreadErrors = true
					}
				}
				currentArgIndex += 1
			}

			if !hasSpreadErrors {
				// Calculate effective argument count, expanding spread elements
				actualArgCount := c.calculateEffectiveArgCount(node.Arguments)

				if constructorSig.IsVariadic {
					debugPrintf("// [Checker NewExpression] Taking VARIADIC branch\n")
					// Variadic constructor
					minExpectedArgs := len(constructorSig.ParameterTypes)
					if len(constructorSig.OptionalParams) == len(constructorSig.ParameterTypes) {
						for i := len(constructorSig.ParameterTypes) - 1; i >= 0; i-- {
							if constructorSig.OptionalParams[i] {
								minExpectedArgs--
							} else {
								break
							}
						}
					}

					if actualArgCount < minExpectedArgs {
						c.addError(node, fmt.Sprintf("Constructor expected at least %d arguments but got %d.", minExpectedArgs, actualArgCount))
					} else {
						// Check fixed arguments
						fixedArgsOk := c.checkFixedArgumentsWithSpread(node.Arguments, constructorSig.ParameterTypes, constructorSig.IsVariadic)

						// Check variadic arguments
						if fixedArgsOk && constructorSig.RestParameterType != nil {
							arrayType, isArray := constructorSig.RestParameterType.(*types.ArrayType)
							if !isArray {
								c.addError(node, fmt.Sprintf("internal checker error: variadic parameter type must be an array type, got %s", constructorSig.RestParameterType.String()))
							} else {
								variadicElementType := arrayType.ElementType
								if variadicElementType == nil {
									variadicElementType = types.Any
								}
								// Check remaining arguments against the element type
								for i := len(constructorSig.ParameterTypes); i < len(node.Arguments); i++ {
									argNode := node.Arguments[i]

									if spreadElement, isSpread := argNode.(*parser.SpreadElement); isSpread {
										if !c.validateSpreadArgument(spreadElement, true, []types.Type{}, 0) {
											continue
										}

										c.visit(spreadElement.Argument)
										argType := spreadElement.Argument.GetComputedType()
										if argType == nil {
											continue
										}

										if !types.IsAssignable(argType, constructorSig.RestParameterType) {
											c.addError(spreadElement, fmt.Sprintf("spread argument: cannot assign type '%s' to rest parameter type '%s'", argType.String(), constructorSig.RestParameterType.String()))
										}
									} else {
										c.visitWithContext(argNode, &ContextualType{
											ExpectedType: variadicElementType,
											IsContextual: true,
										})

										argType := argNode.GetComputedType()
										if argType == nil {
											continue
										}

										if !types.IsAssignable(argType, variadicElementType) {
											c.addError(argNode, fmt.Sprintf("variadic argument %d: cannot assign type '%s' to parameter element type '%s'", i+1, argType.String(), variadicElementType.String()))
										}
									}
								}
							}
						}
					}
				} else {
					debugPrintf("// [Checker NewExpression] Taking NON-VARIADIC branch\n")
					// Non-variadic constructor
					expectedArgCount := len(constructorSig.ParameterTypes)
					minRequiredArgs := expectedArgCount
					if len(constructorSig.OptionalParams) == expectedArgCount {
						for i := expectedArgCount - 1; i >= 0; i-- {
							if constructorSig.OptionalParams[i] {
								minRequiredArgs--
							} else {
								break
							}
						}
					}

					if actualArgCount < minRequiredArgs {
						c.addError(node, fmt.Sprintf("Constructor expected at least %d arguments but got %d.", minRequiredArgs, actualArgCount))
					} else if actualArgCount > expectedArgCount {
						c.addError(node, fmt.Sprintf("Constructor expected at most %d arguments but got %d.", expectedArgCount, actualArgCount))
					} else {
						c.checkFixedArgumentsWithSpread(node.Arguments, constructorSig.ParameterTypes, constructorSig.IsVariadic)
					}
				}
			}
		} else {
			// No constructor signature - just visit args
			for _, arg := range node.Arguments {
				c.visit(arg)
			}
		}
	}

	// Determine the result type based on the constructor type (unified ObjectType system)
	var resultType types.Type
	if objType, ok := constructorType.(*types.ObjectType); ok {
		// For unified ObjectType constructors, check if they have constructor signatures
		if len(objType.ConstructSignatures) > 0 {
			// Use the first constructor signature's return type
			resultType = objType.ConstructSignatures[0].ReturnType
		} else {
			// Callable object but no constructor signatures - return any (matches TypeScript behavior)
			resultType = types.Any
		}
	} else if genericType, ok := constructorType.(*types.GenericType); ok {
		// Handle GenericType constructors by checking their body for constructor signatures
		if bodyObjType, bodyOk := genericType.Body.(*types.ObjectType); bodyOk && len(bodyObjType.ConstructSignatures) > 0 {
			// Use the first constructor signature's return type
			resultType = bodyObjType.ConstructSignatures[0].ReturnType
			debugPrintf("// [Checker NewExpression] GenericType constructor result: %s\n", resultType.String())
		} else {
			// Generic type without constructor signatures - return any
			resultType = types.Any
		}
	} else if forwardRef, ok := constructorType.(*types.ForwardReferenceType); ok {
		// Handle forward reference to class being defined (e.g., new Test() within Test class methods)
		debugPrintf("// [Checker NewExpression] ForwardReferenceType constructor: %s\n", forwardRef.ClassName)

		// Try to resolve the forward reference to get the actual constructor type
		if actualType, _, found := c.env.Resolve(forwardRef.ClassName); found {
			debugPrintf("// [Checker NewExpression] Resolved forward reference to: %T = %s\n", actualType, actualType.String())
			if objType, objOk := actualType.(*types.ObjectType); objOk && len(objType.ConstructSignatures) > 0 {
				resultType = objType.ConstructSignatures[0].ReturnType
			} else {
				// If can't resolve to constructor type, return the forward reference as result type
				resultType = forwardRef
			}
		} else {
			// If can't resolve, return the forward reference as result type
			resultType = forwardRef
		}
	} else if constructorType == types.Any {
		// If constructor type is Any, result is also Any
		resultType = types.Any
	} else {
		// Invalid constructor type - now shows the proper instantiated type in error messages
		c.addError(node.Constructor, fmt.Sprintf("'%s' is not a constructor", constructorType.String()))
		resultType = types.Any
	}

	node.SetComputedType(resultType)
}

// checkTypeofExpression checks the type of a typeof expression.
// This handles TypeScript's typeof operator, which returns a string literal
// like "string", "number", "boolean", "undefined", "object", or "function".
func (c *Checker) checkTypeofExpression(node *parser.TypeofExpression) {
	// Special case: typeof with undefined identifier should not error
	// Per ECMAScript spec, typeof is the only operator that doesn't throw ReferenceError for undefined variables
	if ident, ok := node.Operand.(*parser.Identifier); ok {
		// Check if identifier exists
		_, _, found := c.env.Resolve(ident.Value)
		if !found {
			// Identifier doesn't exist - typeof will return "undefined" string literal type
			node.SetComputedType(&types.LiteralType{Value: vm.String("undefined")})
			node.Operand.SetComputedType(types.Any) // Set operand type to Any to avoid errors
			return
		}
	}

	// Visit the operand first to ensure it has a computed type
	c.visit(node.Operand)

	// Get the operand type
	operandType := node.Operand.GetComputedType()
	if operandType == nil {
		// Set a default if operand type is unknown (shouldn't normally happen)
		node.Operand.SetComputedType(types.Any)
		operandType = types.Any
	}

	// Try to get a more precise literal type for the typeof result
	// if the operand type is known
	if operandType != types.Unknown && operandType != types.Any {
		// Get a more precise result using the helper from types package
		resultType := types.GetTypeofResult(operandType)
		if resultType != nil {
			node.SetComputedType(resultType)
			return
		}
	}

	// Default to the general union of all possible typeof results
	node.SetComputedType(types.TypeofResultType)
}

// checkTypeAssertionExpression handles type assertion expressions (value as Type)
func (c *Checker) checkTypeAssertionExpression(node *parser.TypeAssertionExpression) {
	// Visit the expression being asserted
	c.visit(node.Expression)
	sourceType := node.Expression.GetComputedType()
	if sourceType == nil {
		sourceType = types.Any
	}

	// Resolve the target type
	targetType := c.resolveTypeAnnotation(node.TargetType)
	if targetType == nil {
		c.addError(node.TargetType, "invalid type in type assertion")
		node.SetComputedType(types.Any)
		return
	}

	// Validate the type assertion according to TypeScript rules
	if !c.isValidTypeAssertion(sourceType, targetType) {
		c.addError(node, fmt.Sprintf("conversion of type '%s' to type '%s' may be a mistake because neither type sufficiently overlaps with the other",
			sourceType.String(), targetType.String()))
	}

	// The result type is always the target type
	node.SetComputedType(targetType)
}

// checkSatisfiesExpression handles satisfies expressions (value satisfies Type)
func (c *Checker) checkSatisfiesExpression(node *parser.SatisfiesExpression) {
	// Visit the expression being validated
	c.visit(node.Expression)
	sourceType := node.Expression.GetComputedType()
	if sourceType == nil {
		sourceType = types.Any
	}

	// Resolve the target type
	targetType := c.resolveTypeAnnotation(node.TargetType)
	if targetType == nil {
		c.addError(node.TargetType, "invalid type in satisfies expression")
		node.SetComputedType(types.Any)
		return
	}

	// For satisfies, we need strict checking including excess property checks for object literals
	if objectLit, ok := node.Expression.(*parser.ObjectLiteral); ok {
		// Special handling for object literals - check for excess properties
		c.checkObjectLiteralSatisfies(objectLit, targetType, node)
	} else {
		// For non-object literals, use regular assignability check
		if !types.IsAssignable(sourceType, targetType) {
			c.addError(node, fmt.Sprintf("type '%s' does not satisfy the constraint '%s'",
				sourceType.String(), targetType.String()))
		}
	}

	// The result type is the ORIGINAL expression type, NOT the target type
	// This is the key difference from type assertions
	node.SetComputedType(sourceType)
}

// checkObjectLiteralSatisfies performs strict checking for object literals in satisfies expressions
func (c *Checker) checkObjectLiteralSatisfies(objectLit *parser.ObjectLiteral, targetType types.Type, satisfiesNode *parser.SatisfiesExpression) {
	sourceType := objectLit.GetComputedType()
	if sourceType == nil {
		return
	}

	// First check if the source type is assignable to the target type
	if !types.IsAssignable(sourceType, targetType) {
		c.addError(satisfiesNode, fmt.Sprintf("type '%s' does not satisfy the constraint '%s'",
			sourceType.String(), targetType.String()))
		return
	}

	// For object literals with satisfies, we need to check for excess properties
	// This is stricter than regular assignment
	sourceObjType, sourceIsObj := sourceType.(*types.ObjectType)
	targetObjType, targetIsObj := targetType.(*types.ObjectType)

	if sourceIsObj && targetIsObj {
		// Check for excess properties in the source object literal
		for propName := range sourceObjType.Properties {
			if _, exists := targetObjType.Properties[propName]; !exists {
				// This is an excess property - satisfies should reject it
				c.addError(satisfiesNode, fmt.Sprintf("Object literal may only specify known properties, and '%s' does not exist in type '%s'",
					propName, targetType.String()))
			}
		}
	}
}

// isValidTypeAssertion checks if a type assertion is valid according to TypeScript rules
func (c *Checker) isValidTypeAssertion(sourceType, targetType types.Type) bool {
	// Allow any assertion involving 'any' or 'unknown'
	if sourceType == types.Any || sourceType == types.Unknown ||
		targetType == types.Any || targetType == types.Unknown {
		return true
	}

	// Check if either type is assignable to the other
	if types.IsAssignable(targetType, sourceType) || types.IsAssignable(sourceType, targetType) {
		return true
	}

	// Check for obvious mismatches between primitive types
	// TypeScript allows assertions between primitives only if there's some potential overlap
	if c.isPrimitiveType(sourceType) && c.isPrimitiveType(targetType) {
		// Disallow assertions between completely different primitive types
		if sourceType != targetType {
			return false
		}
	}

	// Allow other assertions (interfaces, objects, etc.) as they might have overlap
	return true
}

// isPrimitiveType checks if a type is a primitive type
func (c *Checker) isPrimitiveType(t types.Type) bool {
	return t == types.String || t == types.Number || t == types.Boolean ||
		t == types.Null || t == types.Undefined
}

// checkInOperator handles type checking for the 'in' operator ("prop" in obj)
func (c *Checker) checkInOperator(leftType, rightType types.Type, node *parser.InfixExpression) {
	// Left operand (property name) should be string, number, or symbol
	// For now, we'll focus on string and number (symbol support can be added later)
	if leftType != types.Any && leftType != types.String && leftType != types.Number {
		// Check if it's a literal string or number type
		if !c.isStringOrNumberLiteralType(leftType) {
			c.addError(node.Left, fmt.Sprintf("the left-hand side of 'in' must be of type 'string' or 'number', but got '%s'", leftType.String()))
		}
	}

	// Right operand (object) should be an object type
	if rightType != types.Any && !c.isObjectType(rightType) {
		c.addError(node.Right, fmt.Sprintf("the right-hand side of 'in' must be an object, but got '%s'", rightType.String()))
	}
}

// isStringOrNumberLiteralType checks if a type is a string or number literal type
func (c *Checker) isStringOrNumberLiteralType(t types.Type) bool {
	// Check for literal types (e.g., "hello", 42)
	if lit, ok := t.(*types.LiteralType); ok {
		// Determine the base type from the literal value
		valueType := lit.Value.Type()
		return valueType == vm.TypeString || valueType == vm.TypeFloatNumber || valueType == vm.TypeIntegerNumber
	}
	return false
}

// isObjectType checks if a type represents an object (not primitive)
func (c *Checker) isObjectType(t types.Type) bool {
	switch typ := t.(type) {
	case *types.ObjectType, *types.ArrayType:
		return true
	case *types.UnionType:
		// For union types with the 'in' operator, we should allow it if ANY member is an object
		// because at runtime, the 'in' check will only happen on the actual object members
		for _, memberType := range typ.Types {
			if c.isObjectType(memberType) {
				return true
			}
		}
		return false
	case *types.TypeParameterType:
		// Type parameters could be objects, allow them (this might need refinement)
		return true
	default:
		// Check for any type that could represent an object
		return t == types.Any
	}
}

// checkInstanceofOperator checks the instanceof operator usage
func (c *Checker) checkInstanceofOperator(leftType, rightType types.Type, node *parser.InfixExpression) {
	// Left operand can be any value (the object to check)
	// No specific type checking needed for left operand

	// Right operand must be a constructor function
	if rightType != types.Any && !c.isConstructorType(rightType) {
		c.addError(node.Right, fmt.Sprintf("Cannot use '%s' as a constructor.", rightType.String()))
	}
}

// isConstructorType checks if a type represents a constructor function
func (c *Checker) isConstructorType(t types.Type) bool {
	if objType, ok := t.(*types.ObjectType); ok {
		// In TypeScript, any callable type can be used as a constructor with 'new'
		// unless it explicitly has no construct signatures
		return objType.IsCallable()
	}
	return false
}

// tryGetConstantStringValue attempts to resolve an identifier to a constant string value
func (c *Checker) tryGetConstantStringValue(ident *parser.Identifier) string {
	// TODO: Implement proper constant value tracking
	// For now, this is a placeholder that returns empty string
	// In a full implementation, we'd track constant assignments and evaluate them
	return ""
}

// instantiateGenericMethod instantiates a generic method with concrete type arguments
func (c *Checker) instantiateGenericMethod(methodType types.Type, elementType types.Type) types.Type {
	// If the method type is a generic type, instantiate it with the element type
	if genericType, ok := methodType.(*types.GenericType); ok {
		var typeArgs []types.Type

		// FIXME why is this hardcoded?
		if genericType.Name == "map" && len(genericType.TypeParameters) == 2 {
			// For map<T, U>, we only provide T (element type)
			// U will be inferred from the callback return type later
			typeArgs = []types.Type{elementType, types.Any} // T = elementType, U = Any for now
		} else {
			// For other methods with single type parameter T
			typeArgs = []types.Type{elementType}
		}

		instantiated := &types.InstantiatedType{
			Generic:       genericType,
			TypeArguments: typeArgs,
		}
		return instantiated.Substitute()
	}

	// If it's not generic, return as-is
	return methodType
}

// checkYieldExpression handles type checking for yield expressions in generator functions
func (c *Checker) checkYieldExpression(node *parser.YieldExpression) {
	// TODO: Check if we're currently in a generator function context
	// For now, we'll allow yield expressions and assign them a generic type

	// 1. Check the yielded value (if present)
	var yieldedType types.Type = types.Undefined
	if node.Value != nil {
		c.visit(node.Value)
		valueType := node.Value.GetComputedType()
		if valueType == nil {
			valueType = types.Any
		}

		if node.Delegate {
			// yield* delegation - the value must be iterable
			// Check if the value has Symbol.iterator method
			// Check if the type has Symbol.iterator property
			debugPrintf("// [Checker yield*] Checking if valueType %T (%s) is iterable\n", valueType, valueType.String())

			// Special case: if the value comes from a generator function call, assume it's iterable
			// This handles cases where generator functions haven't been fully resolved yet in multi-pass checking
			if callExpr, ok := node.Value.(*parser.CallExpression); ok {
				if ident, ok := callExpr.Function.(*parser.Identifier); ok {
					// Check if this identifier refers to a generator function
					// Look for the function declaration in the checker's state
					if c.generatorFunctions[ident.Value] {
						debugPrintf("// [Checker yield*] Detected generator function call, assuming iterable\n")
						yieldedType = types.Any // Safe fallback
						goto setYieldedType
					}
				}
			}

			// Resolve Symbol.iterator via computed path; avoid stringizing here
			iteratorMethodType := c.getPropertyTypeFromType(valueType, "__COMPUTED_PROPERTY__", false)
			debugPrintf("// [Checker yield*] Symbol.iterator method type: %T (%s)\n", iteratorMethodType,
				func() string {
					if iteratorMethodType != nil {
						return iteratorMethodType.String()
					} else {
						return "nil"
					}
				}())

			if iteratorMethodType != nil && iteratorMethodType != types.Never {
				// The value has Symbol.iterator - it's iterable
				// Check if it's a function that returns an Iterator<T>
				if genericType, ok := iteratorMethodType.(*types.GenericType); ok {
					// It's a generic method - we need to instantiate it for the array element type
					// For arrays, instantiate with the element type
					if arrayType, ok := valueType.(*types.ArrayType); ok {
						instantiated := &types.InstantiatedType{
							Generic:       genericType,
							TypeArguments: []types.Type{arrayType.ElementType},
						}
						iteratorMethodType = instantiated.Substitute()
					}
				}

				if objType, ok := iteratorMethodType.(*types.ObjectType); ok && len(objType.CallSignatures) > 0 {
					// Extract the return type of Symbol.iterator method
					signature := objType.CallSignatures[0]
					returnType := signature.ReturnType

					// The return type should be Iterator<T>
					// Try to extract T from the iterator
					if instType, ok := returnType.(*types.InstantiatedType); ok {
						if instType.Generic != nil && instType.Generic.Name == "Iterator" && len(instType.TypeArguments) > 0 {
							// Extract T from Iterator<T>
							yieldedType = instType.TypeArguments[0]
						} else {
							// Fallback - try to extract from known types
							switch vt := valueType.(type) {
							case *types.ArrayType:
								yieldedType = vt.ElementType
							case *types.InstantiatedType:
								if vt.Generic != nil && vt.Generic.Name == "Generator" && len(vt.TypeArguments) > 0 {
									yieldedType = vt.TypeArguments[0]
								} else {
									yieldedType = types.Any
								}
							default:
								yieldedType = types.Any
							}
						}
					} else {
						// Fallback - try to extract from known types
						switch vt := valueType.(type) {
						case *types.ArrayType:
							yieldedType = vt.ElementType
						default:
							yieldedType = types.Any
						}
					}
				} else {
					c.addError(node, "yield* expression must be an iterable")
					yieldedType = types.Any
				}
			} else {
				c.addError(node, "yield* expression must be an iterable")
				yieldedType = types.Any
			}
		} else {
			// Regular yield expression
			yieldedType = valueType
		}
	}

setYieldedType:
	// 2. For now, set the computed type to the yielded value type
	// In a full implementation, this would be IteratorResult<T, TReturn>
	// but for basic functionality, we'll use the yielded type
	node.SetComputedType(yieldedType)

	// 3. Add validation that yield can only be used in generator functions
	// TODO: Implement generator function context tracking
	// For now, we'll just allow it and let the compiler handle validation

	// 4. Collect yield type for generator type inference (similar to return type collection)
	if c.currentInferredYieldTypes != nil {
		c.currentInferredYieldTypes = append(c.currentInferredYieldTypes, yieldedType)
	}

	// Debug commented out
	// fmt.Fprintf(os.Stderr, "// [Checker YieldExpression] %s type: %s\n",
	//   func() string { if node.Delegate { return "yield* delegation" } else { return "yield" } }(),
	//   yieldedType.String())
}

// checkAwaitExpression checks an await expression and unwraps the Promise type
func (c *Checker) checkAwaitExpression(node *parser.AwaitExpression) {
	// 1. Check the argument expression
	if node.Argument != nil {
		c.visit(node.Argument)
		argType := node.Argument.GetComputedType()

		// 2. Unwrap Promise<T> to get T
		if argType != nil {
			// Check if this is a Promise<T> type (InstantiatedType before substitution)
			if instType, ok := argType.(*types.InstantiatedType); ok {
				if instType.Generic != nil && instType.Generic.Name == "Promise" {
					// Extract the inner type T from Promise<T>
					if len(instType.TypeArguments) > 0 {
						innerType := instType.TypeArguments[0]
						node.SetComputedType(innerType)
						debugPrintf("// [Checker AwaitExpression] Unwrapped Promise<%s> to %s\n",
							innerType.String(), innerType.String())
						return
					}
				}
			}

			// Check if this is a Promise-shaped object (after substitution)
			// Promise objects have .then(), .catch(), .finally() methods
			// The .then() callback's first parameter type is the resolved value
			if objType, ok := argType.(*types.ObjectType); ok {
				if thenProp, hasThen := objType.Properties["then"]; hasThen {
					// Extract the type from the .then() callback parameter
					if thenFuncType, ok := thenProp.(*types.ObjectType); ok {
						if len(thenFuncType.CallSignatures) > 0 {
							sig := thenFuncType.CallSignatures[0]
							// The first parameter should be a function that receives the resolved value
							if len(sig.ParameterTypes) > 0 {
								callbackType := sig.ParameterTypes[0]
								if callbackFuncType, ok := callbackType.(*types.ObjectType); ok {
									if len(callbackFuncType.CallSignatures) > 0 {
										callbackSig := callbackFuncType.CallSignatures[0]
										if len(callbackSig.ParameterTypes) > 0 {
											// This is the resolved value type
											resolvedType := callbackSig.ParameterTypes[0]
											node.SetComputedType(resolvedType)
											debugPrintf("// [Checker AwaitExpression] Unwrapped Promise-shaped object to %s\n",
												resolvedType.String())
											return
										}
									}
								}
							}
						}
					}
				}
			}

			// If not a Promise type, await still accepts the value and returns it
			// This matches TypeScript behavior where await can be used on non-Promise values
			node.SetComputedType(argType)
			debugPrintf("// [Checker AwaitExpression] Non-Promise type %s, returning as-is\n",
				argType.String())
		} else {
			// No type information, default to any
			node.SetComputedType(types.Any)
		}
	} else {
		// No argument (shouldn't happen in valid code)
		node.SetComputedType(types.Undefined)
	}
}

// isIterableBySymbolIterator returns true if the type has a computed Symbol.iterator method.
func (c *Checker) isIterableBySymbolIterator(t types.Type) bool {
	if t == nil {
		return false
	}
	// We treat presence of a computed Symbol.iterator property of any type as iterable.
	// The runtime will validate calling it.
	methodType := c.getPropertyTypeFromType(t, "__COMPUTED_PROPERTY__", false)
	return methodType != nil && methodType != types.Never
}

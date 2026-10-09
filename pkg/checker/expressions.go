package checker

import (
	"fmt"
	"github.com/nooga/paserati/pkg/errors"
	"strings"

	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

func (c *Checker) validateComputedPropertyNameType(expr parser.Expression, errorNode parser.Node) {
	keyExprType := expr.GetComputedType()
	if keyExprType == nil {
		return
	}
	if !c.isValidComputedPropertyNameType(keyExprType) {
		c.addErrorWithCode(errorNode, errors.TS2464, "A computed property name must be of type 'string', 'number', 'symbol', or 'any'.")
	}
}

func (c *Checker) isValidComputedPropertyNameType(t types.Type) bool {
	if t == nil {
		return true
	}
	if t == types.Any || t == types.String || t == types.Number || t == types.Symbol {
		return true
	}
	if literal, ok := t.(*types.LiteralType); ok {
		switch literal.Value.Type() {
		case vm.TypeString, vm.TypeIntegerNumber, vm.TypeFloatNumber, vm.TypeBigInt:
			return true
		}
		return false
	}
	if union, ok := t.(*types.UnionType); ok {
		for _, member := range union.Types {
			if !c.isValidComputedPropertyNameType(types.GetWidenedType(member)) {
				return false
			}
		}
		return true
	}
	return false
}

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
			elemType = stripReadonlyArrays(elemType)
			if arrayType, isArray := elemType.(*types.ArrayType); isArray {
				// Use the element type of the spread array
				generalizedType := types.DeeplyWidenType(arrayType.ElementType)
				generalizedElementTypes = append(generalizedElementTypes, generalizedType)
			} else if c.isSpreadableIterableType(elemType) {
				generalizedElementTypes = append(generalizedElementTypes, types.DeeplyWidenType(c.getSpreadElementType(elemType)))
			} else if elemType == types.Any {
				// Spread of 'any' contributes 'any' element type
				generalizedElementTypes = append(generalizedElementTypes, types.Any)
			} else {
				// Error case - spread of non-array: issue a clear compile error
				c.reportNotIterable(spreadElem.Argument, elemType)
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

		// A literal of the wrong length is still a tuple (`[string, number,
		// number]`), which is what the mismatch then reports.
		if tooShort := len(node.Elements) < minRequired; tooShort || (maxAllowed >= 0 && len(node.Elements) > maxAllowed) {
			hasSpread := false
			for _, elem := range node.Elements {
				if _, isSpread := elem.(*parser.SpreadElement); isSpread {
					hasSpread = true
				}
			}
			if !hasSpread {
				elemTypes := make([]types.Type, len(node.Elements))
				for i, elemNode := range node.Elements {
					var ctx *ContextualType
					if i < len(tupleType.ElementTypes) {
						ctx = &ContextualType{ExpectedType: tupleType.ElementTypes[i], IsContextual: true}
					}
					if ctx != nil {
						c.visitWithContext(elemNode, ctx)
					} else {
						c.visit(elemNode)
					}
					et := elemNode.GetComputedType()
					if et == nil {
						et = types.Any
					}
					elemTypes[i] = types.GetWidenedType(et)
				}
				node.SetComputedType(&types.TupleType{ElementTypes: elemTypes})
				return
			}
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
				spreadType = stripReadonlyArrays(spreadType)

				// Validate that the spread array's element type is assignable to expected element type
				if spreadArrayType, isArray := spreadType.(*types.ArrayType); isArray {
					if !types.IsAssignable(spreadArrayType.ElementType, arrayType.ElementType) {
						c.addErrorAtStart(elemNode, errors.TS2322, fmt.Sprintf("Type '%s' is not assignable to type '%s'.",
							spreadArrayType.ElementType.String(), arrayType.ElementType.String()))
					}
				} else if c.isSpreadableIterableType(spreadType) {
					spreadElementType := c.getSpreadElementType(spreadType)
					if !types.IsAssignable(spreadElementType, arrayType.ElementType) {
						c.addErrorAtStart(elemNode, errors.TS2322, fmt.Sprintf("Type '%s' is not assignable to type '%s'.",
							spreadElementType.String(), arrayType.ElementType.String()))
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

				if !c.assignableToFresh(elemNode, actualElemType, arrayType.ElementType) {
					c.reportNotAssignable(elemNode, elemNode, actualElemType, arrayType.ElementType, headAssign)
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

// checkObjectLiteralWithContext handles contextual typing for object literals
// This allows array literals in property values to be typed as tuples when expected
func (c *Checker) checkObjectLiteralWithContext(node *parser.ObjectLiteral, context *ContextualType) {
	expectedType := context.ExpectedType
	debugPrintf("// [Checker ObjectLitContext] Expected type: %T (%s)\n", expectedType, expectedType.String())

	// Try to get the expected type as an ObjectType
	var expectedObjType *types.ObjectType
	switch et := expectedType.(type) {
	case *types.ObjectType:
		expectedObjType = et
	case *types.MappedType:
		// Expand mapped type to ObjectType if possible
		expanded := c.expandMappedType(et)
		if objType, ok := expanded.(*types.ObjectType); ok {
			expectedObjType = objType
		}
	}

	// If we couldn't get an ObjectType, fall back to regular checking
	if expectedObjType == nil {
		debugPrintf("// [Checker ObjectLitContext] Expected type is not an object type. Falling back to regular checking.\n")
		c.checkObjectLiteral(node)
		return
	}

	debugPrintf("// [Checker ObjectLitContext] Using object type with %d properties\n", len(expectedObjType.Properties))

	// Process the object literal with contextual typing for property values
	fields := make(map[string]types.Type)
	seenKeys := make(map[string]bool)
	seenProtoColon := false

	for _, prop := range node.Properties {
		var keyName string

		switch key := prop.Key.(type) {
		case *parser.Identifier:
			keyName = key.Value
		case *parser.StringLiteral:
			keyName = key.Value
		case *parser.NumberLiteral:
			keyName = fmt.Sprintf("%v", key.Value)
		case *parser.BigIntLiteral:
			keyName = key.Value
		case *parser.SpreadElement:
			// Handle spread - visit and merge properties
			c.visit(key.Argument)
			argType := key.Argument.GetComputedType()
			if argType == nil {
				argType = types.Any
			}
			widenedType := types.GetWidenedType(argType)
			if spreadObjType, ok := widenedType.(*types.ObjectType); ok {
				for propName, propType := range spreadObjType.Properties {
					fields[propName] = propType
				}
			}
			continue
		case *parser.ComputedPropertyName:
			c.visit(key.Expr)
			if literal, ok := key.Expr.(*parser.StringLiteral); ok {
				keyName = literal.Value
			} else if literal, ok := key.Expr.(*parser.NumberLiteral); ok {
				keyName = fmt.Sprintf("%v", literal.Value)
			} else {
				keyName = "__COMPUTED_PROPERTY__"
			}
		default:
			keyName = "__UNKNOWN_KEY__"
		}

		// Check for duplicate keys
		isShorthand := false
		if keyIdent, keyOk := prop.Key.(*parser.Identifier); keyOk {
			if valIdent, valOk := prop.Value.(*parser.Identifier); valOk {
				isShorthand = (keyIdent == valIdent || keyIdent.Value == valIdent.Value)
			}
		}

		if keyName != "__COMPUTED_PROPERTY__" && keyName != "__UNKNOWN_KEY__" {
			isProtoColon := (keyName == "__proto__" && !isShorthand)
			if seenKeys[keyName] && keyName != "__proto__" {
				c.addErrorWithCode(prop.Key, errors.TS1117, "An object literal cannot have multiple properties with the same name.")
			}
			if isProtoColon && seenProtoColon {
				c.addError(prop.Key, "duplicate __proto__ fields are not allowed")
			}
			if isProtoColon {
				seenProtoColon = true
			}
			seenKeys[keyName] = true
		}

		// Visit the property value with context if we have an expected type for this property
		expectedPropType := expectedObjType.Properties[keyName]
		if expectedPropType != nil {
			debugPrintf("// [Checker ObjectLitContext] Property '%s' has expected type: %s\n", keyName, expectedPropType.String())
			c.visitWithContext(prop.Value, &ContextualType{
				ExpectedType: expectedPropType,
				IsContextual: true,
			})
		} else {
			// No expected type for this property - regular visit
			c.visit(prop.Value)
		}

		valueType := prop.Value.GetComputedType()
		if valueType == nil {
			valueType = types.Any
		}

		// Handle __proto__: value specially
		if keyName == "__proto__" && !isShorthand {
			widenedProtoType := types.GetWidenedType(valueType)
			if protoObjType, ok := widenedProtoType.(*types.ObjectType); ok {
				for propName, propType := range protoObjType.Properties {
					if _, exists := fields[propName]; !exists {
						fields[propName] = propType
					}
				}
			}
		} else {
			fields[keyName] = valueType
		}
	}

	// Create the result type
	resultType := &types.ObjectType{Properties: fields}
	node.SetComputedType(resultType)
	debugPrintf("// [Checker ObjectLitContext] Result type: %s\n", resultType.String())
}

// mergeSpreadOperand merges a spread operand's properties into fields,
// returning false if the type can't be spread at all. A union (typically
// the result of `cond && {...}`) distributes: each object member merges its
// properties, and a falsy/nullish member — the short-circuit branch of the
// `&&`/`||`/`??` that produced the union — contributes nothing, exactly as
// it would at runtime.
func (c *Checker) mergeSpreadOperand(fields map[string]types.Type, t types.Type) bool {
	switch spreadType := t.(type) {
	case *types.ObjectType:
		debugPrintf("// [Checker ObjectLit Spread] Merging properties from spread object: %s\n", spreadType.String())
		for propName, propType := range spreadType.Properties {
			fields[propName] = propType // Later properties override earlier ones
			debugPrintf("// [Checker ObjectLit Spread] Added property '%s': %s\n", propName, propType.String())
		}
		return true
	case *types.ArrayType:
		// Arrays can be spread but only add numeric indices and length.
		// For simplicity, we'll allow this but not add specific properties.
		debugPrintf("// [Checker ObjectLit Spread] Spreading array type (no properties added)\n")
		return true
	case *types.UnionType:
		for _, member := range spreadType.Types {
			if !c.mergeSpreadOperand(fields, member) {
				return false
			}
		}
		return true
	case *types.TypeParameterType:
		// Spreading a type parameter is allowed whatever its constraint
		// (#616); the constraint's known properties carry over when it has
		// object shape.
		if p := spreadType.Parameter; p != nil && p.Constraint != nil {
			c.mergeSpreadOperand(fields, types.GetWidenedType(p.Constraint))
		}
		return true
	case *types.LiteralType:
		// A literal member of a union only ever shows up here as the falsy
		// branch a logical operator retained (e.g. `false` from `cnd && {}`);
		// it contributes no properties, same as spreading `undefined`.
		if !spreadType.Value.IsTruthy() {
			return true
		}
		return false
	default:
		// Allow undefined (from yield without argument), any (can't verify
		// statically), null and boolean (the other short-circuit results
		// `&&`/`||`/`??` can retain).
		// `object` (NonPrimitive) is spreadable too, with no known properties.
		if t == types.Any || t == types.Undefined || t == types.Null || t == types.Boolean || t == types.NonPrimitive {
			debugPrintf("// [Checker ObjectLit Spread] Spreading any/undefined/null/boolean type (no properties added)\n")
			return true
		}
		return false
	}
}

// checkObjectLiteral checks the type of an object literal expression.
func (c *Checker) checkObjectLiteral(node *parser.ObjectLiteral) {
	fields := make(map[string]types.Type)
	seenKeys := make(map[string]bool)
	seenProtoColon := false // Track if we've seen __proto__: value (colon syntax)

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
		case *parser.BigIntLiteral: // Allow bigint literals as keys, convert to string
			// Note: JavaScript converts bigint keys to strings internally
			keyName = key.Value // BigIntLiteral.Value is already a string
		case *parser.SpreadElement:
			// Handle spread syntax: {...obj}
			// Check that the argument is an object type
			c.visit(key.Argument)
			argType := key.Argument.GetComputedType()
			if argType == nil {
				argType = types.Any
			}

			// Check if the type can be spread (is an object type)
			widenedType := c.apparentType(types.GetWidenedType(argType))
			if !c.mergeSpreadOperand(fields, widenedType) {
				c.addErrorWithCode(key.Argument, errors.TS2698, "Spread types may only be created from object types.")
			}
			// Skip the rest of the property processing for spread elements
			continue
		case *parser.ComputedPropertyName:
			// Handle computed properties: [expression]
			c.visit(key.Expr)
			// TS2464: computed property name must be string, number, symbol, or any.
			// Keep object literal checks conservative because JavaScript object literal
			// keys support runtime ToPropertyKey coercion and the smoke suite covers it.
			if keyExprType := key.Expr.GetComputedType(); keyExprType != nil {
				widenedKeyType := types.GetWidenedType(keyExprType)
				switch widenedKeyType {
				case types.Boolean, types.Null, types.Undefined, types.Void, types.Never:
					c.addErrorWithCode(key, errors.TS2464, "A computed property name must be of type 'string', 'number', 'symbol', or 'any'.")
				}
			}

			// Note: keyType is computed but primarily used for validation
			// The actual key name is determined at runtime for computed properties

			// Try to get a compile-time constant key if possible
			if literal, ok := key.Expr.(*parser.StringLiteral); ok {
				keyName = literal.Value
			} else if literal, ok := key.Expr.(*parser.NumberLiteral); ok {
				keyName = fmt.Sprintf("%v", literal.Value)
			} else if literal, ok := key.Expr.(*parser.BigIntLiteral); ok {
				keyName = literal.Value
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

		// Detect if this is shorthand property syntax ({x} instead of {x: value})
		isShorthand := false
		if keyIdent, keyOk := prop.Key.(*parser.Identifier); keyOk {
			if valIdent, valOk := prop.Value.(*parser.Identifier); valOk {
				// Shorthand if same identifier (by reference) or same name
				isShorthand = (keyIdent == valIdent || keyIdent.Value == valIdent.Value)
			}
		}

		// Check for duplicate keys (but skip for computed keys since they can be dynamic)
		// Allow getters/setters to share the same key by checking if the value is a MethodDefinition
		if keyName != "__COMPUTED_PROPERTY__" && keyName != "__UNKNOWN_KEY__" {
			// Per Annex B.3.1: duplicate __proto__ is only an error when both use colon syntax
			// Shorthand {__proto__} creates a regular property and can be duplicated
			isProtoColon := (keyName == "__proto__" && !isShorthand)

			// Check if this is a getter/setter MethodDefinition
			if methodDef, isMethodDef := prop.Value.(*parser.MethodDefinition); isMethodDef {
				if methodDef.Kind == "getter" || methodDef.Kind == "setter" {
					// Getters and setters can share the same key name - don't mark as duplicate
					// But do track them to prevent multiple getters or multiple setters for same key
					// For now, be lenient and allow it - JavaScript allows redefining getters/setters
				} else {
					// Regular method - check for conflicts
					if seenKeys[keyName] && keyName != "__proto__" {
						c.addErrorWithCode(prop.Key, errors.TS1117, "An object literal cannot have multiple properties with the same name.")
					}
					if isProtoColon && seenProtoColon {
						c.addError(prop.Key, "duplicate __proto__ fields are not allowed")
					}
					if isProtoColon {
						seenProtoColon = true
					}
					seenKeys[keyName] = true
				}
			} else {
				// Regular property - check for conflicts
				if seenKeys[keyName] && keyName != "__proto__" {
					c.addErrorWithCode(prop.Key, errors.TS1117, "An object literal cannot have multiple properties with the same name.")
				}
				if isProtoColon && seenProtoColon {
					c.addError(prop.Key, "duplicate __proto__ fields are not allowed")
				}
				if isProtoColon {
					seenProtoColon = true
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

						// Special handling for __proto__: only colon syntax sets prototype
						// Per Annex B.3.1, shorthand {__proto__} creates a regular own property
						// (isShorthand was detected earlier in the loop)
						if keyName == "__proto__" && !isShorthand {
							// __proto__: value sets the prototype, so merge its properties into the object type
							widenedProtoType := types.GetWidenedType(valueType)
							if protoObjType, ok := widenedProtoType.(*types.ObjectType); ok {
								// Merge prototype properties into our object
								for propName, propType := range protoObjType.Properties {
									// Only add if not already defined (own properties override prototype)
									if _, exists := fields[propName]; !exists {
										fields[propName] = propType
										preliminaryObjType.SetProperty(propName, propType)
									}
								}
							}
							// Don't add __proto__ itself as a property
						} else {
							fields[keyName] = valueType
							preliminaryObjType.SetProperty(keyName, valueType)
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
		case *parser.BigIntLiteral:
			keyName = key.Value
		case *parser.SpreadElement:
			continue // Skip spread elements
		case *parser.ComputedPropertyName:
			// Handle computed properties in the same way as first pass
			if literal, ok := key.Expr.(*parser.StringLiteral); ok {
				keyName = literal.Value
			} else if literal, ok := key.Expr.(*parser.NumberLiteral); ok {
				keyName = fmt.Sprintf("%v", literal.Value)
			} else if literal, ok := key.Expr.(*parser.BigIntLiteral); ok {
				keyName = literal.Value
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
			preliminaryObjType.SetProperty(keyName, types.NewFunctionType(preliminaryFuncSig))
		} else if _, isArrowFunction := prop.Value.(*parser.ArrowFunctionLiteral); isArrowFunction {
			// For arrow functions, we can use a generic function type temporarily
			preliminaryObjType.SetProperty(keyName, types.NewFunctionType(&types.Signature{
				ParameterTypes: []types.Type{}, // We'll refine this later
				ReturnType:     types.Any,
			}))
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
			preliminaryObjType.SetProperty(keyName, types.NewFunctionType(preliminaryFuncSig))
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
					preliminaryObjType.SetProperty(getterName, types.NewFunctionType(preliminaryFuncSig))
					// Also store the property type for type checking
					preliminaryObjType.SetProperty(keyName, preliminaryFuncSig.ReturnType)
				} else if methodDef.Kind == "setter" {
					setterName := "__set__" + keyName
					preliminaryObjType.SetProperty(setterName, types.NewFunctionType(preliminaryFuncSig))
					// Also store the property type for type checking
					if len(preliminaryFuncSig.ParameterTypes) > 0 {
						preliminaryObjType.SetProperty(keyName, preliminaryFuncSig.ParameterTypes[0])
					} else {
						preliminaryObjType.SetProperty(keyName, types.Any)
					}
				} else {
					preliminaryObjType.SetProperty(keyName, types.NewFunctionType(preliminaryFuncSig))
				}
			}
		}
	}

	// Third pass: visit function properties with 'this' context set to the complete preliminary type
	outerThisType := c.currentThisType      // Save outer this context
	outerInObjectMethod := c.inObjectMethod // Save outer object method context
	c.currentThisType = preliminaryObjType  // Set this context to the object being constructed
	c.inObjectMethod = true                 // Mark that we're in an object method (for super support)
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

		// Skip if already processed in first pass (non-function properties).
		// An accessor's pair shares its key, so accessors are never skipped.
		// (Computed-key accessors are fully handled by the first pass.)
		isAccessor := false
		if md, ok := prop.Value.(*parser.MethodDefinition); ok && (md.Kind == "getter" || md.Kind == "setter") {
			_, computedKey := prop.Key.(*parser.ComputedPropertyName)
			isAccessor = !computedKey
		}
		if _, isNonFunction := fields[keyName]; isNonFunction && !isAccessor {
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
			// Arrow functions capture 'this' lexically from enclosing scope, NOT from object literal
			// Temporarily restore outer this type so arrow functions see the class/function 'this'
			debugPrintf("// [Checker ObjectLit] Visiting arrow function property '%s' with outer this context\n", keyName)
			c.currentThisType = outerThisType
			c.visit(prop.Value)
			c.currentThisType = preliminaryObjType
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
			if methodDef.Kind == "setter" && methodDef.Value != nil {
				c.markParametersContextual(methodDef.Value.Parameters, nil) // typed by the getter
			}
			c.visit(prop.Value)
			valueType := prop.Value.GetComputedType()
			if valueType == nil {
				valueType = types.Any
			}

			// Store getter/setter with appropriate prefix to match compiler expectations
			if methodDef.Kind == "getter" {
				// Check that getter has a return statement (TS2378)
				if methodDef.Value != nil && methodDef.Value.Body != nil && !bodyContainsReturn(methodDef.Value.Body) {
					c.addErrorWithCode(methodDef.Key, errors.TS2378, "A 'get' accessor must return a value.")
				}
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
	c.inObjectMethod = outerInObjectMethod
	debugPrintf("// [Checker ObjectLit] Restored this context to: %v\n", outerThisType)

	// Handle dynamic computed properties by creating index signatures.
	computedValueTypes := collectDynamicComputedObjectValueTypes(node)
	hasComputedProperties := len(computedValueTypes) > 0
	finalFields := make(map[string]types.Type)

	for key, valueType := range fields {
		if key == "__COMPUTED_PROPERTY__" {
			// Special-case: if the computed key expression was Symbol.iterator, record an explicit property marker
			// so iterable detection can succeed.
			finalFields["__COMPUTED_PROPERTY__"] = unionTypesOrAny(computedValueTypes, valueType)
		} else {
			finalFields[key] = valueType
		}
	}

	// Create the final ObjectType
	objType := &types.ObjectType{Properties: finalFields}
	// A getter without a setter is a read-only property.
	for key := range finalFields {
		if name, ok := strings.CutPrefix(key, "__get__"); ok {
			if _, hasSetter := finalFields["__set__"+name]; !hasSetter {
				setReadonlyProperty(objType, name, true)
			}
		}
	}

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

func collectDynamicComputedObjectValueTypes(node *parser.ObjectLiteral) []types.Type {
	var valueTypes []types.Type
	for _, prop := range node.Properties {
		if prop == nil || prop.Value == nil {
			continue
		}
		key, ok := prop.Key.(*parser.ComputedPropertyName)
		if !ok || !isDynamicComputedObjectKey(key) {
			continue
		}
		valueType := prop.Value.GetComputedType()
		if valueType == nil {
			valueType = types.Any
		}
		valueTypes = append(valueTypes, valueType)
	}
	return valueTypes
}

func isDynamicComputedObjectKey(key *parser.ComputedPropertyName) bool {
	switch expr := key.Expr.(type) {
	case *parser.StringLiteral, *parser.NumberLiteral, *parser.BigIntLiteral:
		return false
	case *parser.MemberExpression:
		if objectIdent, ok := expr.Object.(*parser.Identifier); ok && objectIdent.Value == "Symbol" {
			return true
		}
	}
	return true
}

func unionTypesOrAny(typeList []types.Type, fallback types.Type) types.Type {
	if len(typeList) == 0 {
		if fallback != nil {
			return fallback
		}
		return types.Any
	}
	if len(typeList) == 1 {
		return typeList[0]
	}
	return types.NewUnionType(typeList...)
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

	// A template without substitutions is a string literal (`abc` has type "abc",
	// widening to string in a mutable location); otherwise the type is string.
	if text, ok := noSubstitutionTemplateText(node); ok {
		node.SetComputedType(&types.LiteralType{Value: vm.String(text)})
		return
	}
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
// instantiateAliasReference instantiates a generic type alias reference
// (`DeepReadonly<{ size: number }>`) through the checker's name-based
// substitution, which covers mapped, conditional and indexed-access bodies.
// nil if the alias isn't a known generic type.
func (c *Checker) instantiateAliasReference(ref *types.GenericTypeAliasForwardReference) types.Type {
	resolved, found := c.env.ResolveType(ref.AliasName)
	if !found {
		return nil
	}
	generic, ok := resolved.(*types.GenericType)
	if !ok || len(ref.TypeArguments) != len(generic.TypeParameters) {
		return nil
	}
	for _, arg := range ref.TypeArguments {
		if arg == nil {
			return nil // a placeholder reference with no recorded arguments
		}
	}
	inst := c.instantiateGenericType(generic, ref.TypeArguments, nil)
	if inst == nil || inst == types.Type(ref) {
		return nil
	}
	return inst
}

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
					propType, found := objType.Properties[propertyName]
					directProperty := found
					if !found {
						for _, sig := range objType.IndexSignatures {
							if sig.KeyType == types.String || sig.KeyType == types.Any {
								propType = sig.ValueType
								found = true
								break
							}
						}
					}
					if found {
						// For optional properties, add undefined to the type
						effectivePropType := propType
						if directProperty && objType.IsPropertyOptional(propertyName) {
							effectivePropType = types.NewUnionType(propType, types.Undefined)
						}
						if unionType, ok := effectivePropType.(*types.UnionType); ok {
							// Compute complement: union minus the narrowed type(s)
							var remainingType types.Type = effectivePropType
							if complementUnion, ok := complementType.(*types.UnionType); ok {
								// Remove each member of the complement union
								for _, ct := range complementUnion.Types {
									if ut, ok := remainingType.(*types.UnionType); ok {
										remainingType = ut.RemoveType(ct)
									}
								}
							} else {
								remainingType = unionType.RemoveType(complementType)
							}
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
							propType, found := objType.Properties[propertyName]
							directProperty := found
							if !found {
								for _, sig := range objType.IndexSignatures {
									if sig.KeyType == types.String || sig.KeyType == types.Any {
										propType = sig.ValueType
										found = true
										break
									}
								}
							}
							if found {
								// For optional properties, add undefined to the type
								effectivePropType := propType
								if directProperty && objType.IsPropertyOptional(propertyName) {
									effectivePropType = types.NewUnionType(propType, types.Undefined)
								}
								if unionType, ok := effectivePropType.(*types.UnionType); ok {
									var remainingType types.Type = effectivePropType
									if complementUnion, ok := complementType.(*types.UnionType); ok {
										for _, ct := range complementUnion.Types {
											if ut, ok := remainingType.(*types.UnionType); ok {
												remainingType = ut.RemoveType(ct)
											}
										}
									} else {
										remainingType = unionType.RemoveType(complementType)
									}
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
	errorsBefore := len(c.errors)
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
	// Reading through `readonly T` is reading T. Unwrap it unless the inner
	// type is one the ReadonlyType case below handles itself (objects, and
	// arrays/tuples, whose mutators readonly hides): a type parameter or
	// mapped type underneath used to be rejected outright (#613).
	for {
		if ref, ok := widenedObjectType.(*types.GenericTypeAliasForwardReference); ok {
			// A recursive generic alias (`DeepReadonly<T[K]>`) left as a
			// forward reference: resolve it now that its arguments are known.
			if resolved := c.instantiateAliasReference(ref); resolved != nil {
				widenedObjectType = types.GetWidenedType(resolved)
				continue
			}
			break
		}
		ro, ok := widenedObjectType.(*types.ReadonlyType)
		if !ok {
			break
		}
		switch ro.InnerType.(type) {
		case *types.ObjectType:
		case *types.ArrayType, *types.TupleType:
			// `readonly T[]` reads like `T[]` minus the mutators (#637).
			if readonlyArrayMutators[propertyName] {
				c.reportPropertyNotFound(node.Property, node.Object, propertyName, ro.String())
				node.SetComputedType(types.Any) // already reported; don't cascade
				return
			}
			widenedObjectType = ro.InnerType
			continue
		default:
			widenedObjectType = types.GetWidenedType(ro.InnerType)
			continue
		}
		break
	}

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
				c.reportPropertyNotFound(node.Property, node.Object, propertyName, "string")
				// resultType remains types.Never
			}
		}
	} else if widenedObjectType == types.Number {
		// Check prototype registry for Number methods
		if methodType := c.env.GetPrimitivePrototypeMethodType("number", propertyName); methodType != nil {
			resultType = methodType
		} else {
			c.reportPropertyNotFound(node.Property, node.Object, propertyName, "number")
			// resultType remains types.Never
		}
	} else if widenedObjectType == types.RegExp {
		// Check prototype registry for RegExp methods and properties
		if methodType := c.env.GetPrimitivePrototypeMethodType("RegExp", propertyName); methodType != nil {
			resultType = methodType
		} else {
			c.reportPropertyNotFound(node.Property, node.Object, propertyName, "RegExp")
			// resultType remains types.Never
		}
	} else if widenedObjectType == types.Symbol {
		// Check prototype registry for Symbol methods and properties
		if methodType := c.env.GetPrimitivePrototypeMethodType("symbol", propertyName); methodType != nil {
			resultType = methodType
		} else {
			c.reportPropertyNotFound(node.Property, node.Object, propertyName, "symbol")
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
					c.reportPropertyNotFound(node.Property, node.Object, propertyName, obj.String())
					// resultType remains types.Never
				}
			}
		case *types.TupleType:
			if propertyName == "length" {
				resultType = types.Number
			} else if methodType := c.env.GetPrimitivePrototypeMethodType("array", propertyName); methodType != nil {
				resultType = c.instantiateGenericMethod(methodType, getTupleElementUnion(obj))
			} else {
				c.reportPropertyNotFound(node.Property, node.Object, propertyName, obj.String())
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
						// Optional properties have type T | undefined
						if obj.IsPropertyOptional(propertyName) {
							resultType = types.NewUnionType(fieldType, types.Undefined)
						}
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

					if resultType == types.Never && propertyName == "prototype" && len(obj.ConstructSignatures) > 0 && obj.ConstructSignatures[0].ReturnType != nil {
						// A class constructor's `prototype` is an instance of the class.
						resultType = obj.ConstructSignatures[0].ReturnType
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
									c.reportPropertyNotFound(node.Property, node.Object, propertyName, obj.String())
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
								c.reportPropertyNotFound(node.Property, node.Object, propertyName, obj.String())
								// resultType remains types.Never
							}
						}
					}
				}
			}
		case *types.IntersectionType:
			// Handle property access on intersection types
			var propType types.Type
			if apparent, ok := c.apparentType(obj).(*types.IntersectionType); ok {
				propType = c.getPropertyTypeFromIntersection(apparent, propertyName)
			} else {
				propType = c.getPropertyTypeFromType(c.apparentType(obj), propertyName, false)
			}
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
					if expandedObj.IsPropertyOptional(propertyName) {
						resultType = types.NewUnionType(propType, types.Undefined)
					}
					debugPrintf("// [Checker MemberExpr] Found property '%s' in expanded type: %s\n", propertyName, propType.String())
				} else {
					// Check index signatures before reporting error
					found := false
					for _, sig := range expandedObj.IndexSignatures {
						if sig.KeyType == types.String || sig.KeyType == types.Any {
							resultType = sig.ValueType
							found = true
							break
						}
					}
					if !found {
						// Check if property is optional
						isOptional := expandedObj.OptionalProperties != nil && expandedObj.OptionalProperties[propertyName]
						if !isOptional {
							c.reportPropertyNotFound(node.Property, node.Object, propertyName, obj.String())
							resultType = types.Never
						} else {
							resultType = types.Undefined
						}
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
						c.addErrorWithCode(node.Property, errors.TS2339, fmt.Sprintf("Property '%s' does not exist on type '%s'.", obj.String(), subst.String()))
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

			// null/undefined members are reported once (TS18047/18048/...) and
			// the access is then checked against what remains.
			var unionMembers []types.Type
			if remaining, ok := c.stripNullishObject(node.Object, obj); !ok {
				unionMembers = nil
				resultType = types.Any
			} else if remainingUnion, isUnion := remaining.(*types.UnionType); isUnion {
				unionMembers = remainingUnion.Types
			} else {
				unionMembers = []types.Type{remaining}
			}
			if len(unionMembers) == 0 {
				break
			}

			for _, memberType := range unionMembers {
				// Create a temporary member expression to check this member type
				memberHasProperty := false
				var memberResultType types.Type

				// Check what type this member would produce for the property
				switch member := memberType.(type) {
				case *types.ObjectType:
					if fieldType, exists := member.Properties[propertyName]; exists {
						memberHasProperty = true
						memberResultType = fieldType
					} else {
						// Check index signatures as fallback
						for _, sig := range member.IndexSignatures {
							if sig.KeyType == types.String || sig.KeyType == types.Any {
								memberHasProperty = true
								memberResultType = sig.ValueType
								break
							}
						}
					}
				case *types.ArrayType:
					if propertyName == "length" {
						memberHasProperty = true
						memberResultType = types.Number
					} else if methodType := c.env.GetPrimitivePrototypeMethodType("array", propertyName); methodType != nil {
						memberHasProperty = true
						memberResultType = c.instantiateGenericMethod(methodType, member.ElementType)
					}
				default:
					// Check primitive prototypes for string, number, boolean, symbol, RegExp
					var prototypeName string
					primMember := memberType
					if lit, ok := memberType.(*types.LiteralType); ok {
						// Literal members expose their primitive's prototype.
						switch {
						case lit.Value.Type() == vm.TypeString:
							primMember = types.String
						case lit.Value.IsNumber():
							primMember = types.Number
						case lit.Value.Type() == vm.TypeBoolean:
							primMember = types.Boolean
						case lit.Value.Type() == vm.TypeBigInt:
							primMember = types.BigInt
						}
					}
					switch primMember {
					case types.BigInt:
						prototypeName = "bigint"
					case types.String:
						prototypeName = "string"
					case types.Number:
						prototypeName = "number"
					case types.Boolean:
						prototypeName = "boolean"
					case types.Symbol:
						prototypeName = "symbol"
					case types.RegExp:
						prototypeName = "RegExp"
					}
					if prototypeName != "" {
						if methodType := c.env.GetPrimitivePrototypeMethodType(prototypeName, propertyName); methodType != nil {
							memberHasProperty = true
							memberResultType = methodType
						}
					}
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
				c.reportPropertyNotFound(node.Property, node.Object, propertyName, types.NewUnionType(unionMembers...).String())
				resultType = types.Never
			}
		case *types.EnumType:
			// Handle property access on enum types (enum member access)
			if memberType, exists := obj.Members[propertyName]; exists {
				resultType = memberType
				debugPrintf("// [Checker MemberExpr] Found enum member '%s.%s': %s\n", obj.Name, propertyName, memberType.String())
			} else {
				c.addErrorWithCode(node.Property, errors.TS2339, fmt.Sprintf("Property '%s' does not exist on type 'typeof %s'.", propertyName, obj.Name))
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
						c.reportPropertyNotFound(node.Property, node.Object, propertyName, resolvedType.String())
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
		case *types.TypeAliasForwardReference:
			// Handle property access on type alias forward reference
			// Resolve the forward reference to the actual type
			debugPrintf("// [Checker MemberExpr] TypeAliasForwardReference: %s, property: %s\n", obj.AliasName, propertyName)
			if resolvedAlias, found := c.env.ResolveType(obj.AliasName); found {
				if _, stillForward := resolvedAlias.(*types.TypeAliasForwardReference); !stillForward {
					// Type has been resolved - recursively check property access
					debugPrintf("// [Checker MemberExpr] Resolved type alias '%s' to: %T = %s\n", obj.AliasName, resolvedAlias, resolvedAlias.String())
					savedObjectType := node.Object.GetComputedType()
					node.Object.SetComputedType(resolvedAlias)
					c.checkMemberExpression(node)
					resultType = node.GetComputedType()
					node.Object.SetComputedType(savedObjectType)
					node.SetComputedType(resultType)
					return // Skip setting the computed type again at the end
				}
			}
			c.addError(node.Object, fmt.Sprintf("could not resolve type alias '%s'", obj.AliasName))
			resultType = types.Never
		case *types.ObjectTypeMarker:
			// typeof x === "object" narrows to object type marker
			// Property access on object type returns any (same as TypeScript)
			resultType = types.Any
		case *types.GenericType:
			// A generic class's constructor: static members live on the body.
			if body, isObj := obj.Body.(*types.ObjectType); isObj {
				if propType, exists := body.Properties[propertyName]; exists {
					resultType = propType
				} else if propertyName == "prototype" || c.classHasExtendsClause(strings.TrimSuffix(obj.Name, "Constructor")) {
					// Statics inherited from a base class are not copied onto the body.
					resultType = types.Any
				} else {
					c.addErrorWithCode(node.Property, errors.TS2339, fmt.Sprintf("Property '%s' does not exist on type 'typeof %s'.", propertyName, strings.TrimSuffix(obj.Name, "Constructor")))
				}
			} else {
				resultType = types.Any
			}
		default:
			// This covers cases where widenedObjectType was not String, Any, ArrayType, ObjectType, etc.
			// e.g., trying to access property on number, boolean, null, undefined
			if widenedObjectType == types.Null || widenedObjectType == types.Undefined {
				c.reportNullishObject(node.Object, widenedObjectType)
				resultType = types.Any
			} else if widenedObjectType == types.Unknown {
				c.reportUnknownObject(node.Object)
				resultType = types.Any
			} else {
				resultType = c.primitivePropertyType(widenedObjectType, propertyName, node)
			}
		}
	}

	// A failed property access has TypeScript's errorType (any-like), which
	// suppresses follow-on diagnostics; never would cascade into bogus errors.
	if resultType == types.Never && len(c.errors) > errorsBefore {
		resultType = types.Any
	}

	// The value read from a readonly property is its plain type: the
	// ReadonlyType wrapper marks the property slot (checked on assignment
	// against the declared type) and means nothing on a primitive value,
	// where it broke arithmetic on `static readonly x = 1`.
	if ro, ok := resultType.(*types.ReadonlyType); ok {
		switch ro.InnerType.(type) {
		case *types.ArrayType, *types.TupleType, *types.ObjectType:
		default:
			resultType = ro.InnerType
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

	// Widen literal types to their base types before checking indexability
	// This allows "lol"[1] to be treated as string[number]
	leftType = types.GetWidenedType(leftType)

	// Expand MappedType (e.g., Record<string, string>) to ObjectType with index signatures
	leftType = c.expandIfMappedType(leftType)

	isIndexStringLiteral := false
	var indexStringValue string
	if litIndex, ok := indexType.(*types.LiteralType); ok && litIndex.Value.Type() == vm.TypeString {
		isIndexStringLiteral = true
		indexStringValue = vm.AsString(litIndex.Value)
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

		case *types.TupleType:
			// Base is TupleType - can index with numeric literal or general number
			if types.IsAssignable(indexType, types.Number) {
				// Check if the index is a numeric literal - if so, we can get the exact element type
				if litIndex, ok := indexType.(*types.LiteralType); ok && litIndex.Value.IsNumber() {
					indexValue := vm.AsNumber(litIndex.Value)
					idx := int(indexValue)
					if idx >= 0 && idx < len(base.ElementTypes) {
						// Valid index into fixed elements
						resultType = base.ElementTypes[idx]
						debugPrintf("// [Checker IndexExpr] Tuple literal index %d -> %s\n", idx, resultType.String())
					} else if base.RestElementType != nil && idx >= len(base.ElementTypes) {
						// Index into rest element
						resultType = base.RestElementType
						debugPrintf("// [Checker IndexExpr] Tuple rest index %d -> %s\n", idx, resultType.String())
					} else {
						if indexValue < 0 {
							c.addErrorWithCode(node.Index, errors.TS2514, "A tuple type cannot be indexed with a negative value.")
						} else {
							c.addErrorWithCode(node.Index, errors.TS2493, fmt.Sprintf("Tuple type '%s' of length '%d' has no element at index '%d'.", base.String(), len(base.ElementTypes), idx))
						}
						resultType = types.Undefined
						debugPrintf("// [Checker IndexExpr] Tuple index %d out of bounds\n", idx)
					}
				} else {
					// General number index - return union of all possible element types
					resultType = getTupleElementUnion(base)
					debugPrintf("// [Checker IndexExpr] Tuple general number index -> %s\n", resultType.String())
				}
			} else if types.IsAssignable(indexType, types.String) || types.IsAssignable(indexType, types.Symbol) {
				// String or Symbol index - accessing tuple properties (like length, Symbol.iterator)
				resultType = types.Any
			} else {
				c.addError(node.Index, fmt.Sprintf("tuple index must be of type number, string, or symbol, got %s", indexType.String()))
				resultType = types.Any
			}

		case *types.ObjectType:
			// Base is ObjectType
			// Index must be string or number (or any)
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
						// Check index signatures before defaulting to any
						found := false
						for _, sig := range base.IndexSignatures {
							if sig.KeyType == types.String || sig.KeyType == types.Any {
								resultType = sig.ValueType
								found = true
								break
							}
						}
						if !found {
							resultType = types.Any
						}
					}
				} else {
					// Index is a general string/number/any - check index signatures
					found := false
					for _, sig := range base.IndexSignatures {
						w := types.GetWidenedType(indexType)
						if sig.KeyType == w || sig.KeyType == types.Any {
							resultType = sig.ValueType
							found = true
							break
						}
					}
					if !found {
						resultType = types.Any
					}
				}
			} else {
				// Invalid index type for object
				c.reportInvalidIndexType(node, indexType)
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
				}
			}

			if allMembersSupported && len(possibleTypes) > 0 {
				// All members support indexing, create union of result types
				resultType = types.NewUnionType(possibleTypes...)
			} else {
				// Some members don't support indexing
				c.reportNonIndexable(node, leftType)
			}

		case *types.Primitive:
			// Allow indexing on strings?
			if base == types.String {
				// 4. Check index type (number for string characters, string/symbol for properties)
				if types.IsAssignable(indexType, types.Number) {
					// Numeric index - accessing string characters
					resultType = types.String
				} else if isIndexStringLiteral && isNumericPropertyName(indexStringValue) {
					// "0" indexes a character just like 0 does
					resultType = types.String
				} else if isIndexStringLiteral {
					resultType = c.getPropertyTypeFromType(base, indexStringValue, false)
					if resultType == types.Never {
						c.addErrorWithCode(node.Index, errors.TS2339, fmt.Sprintf("Property '%s' does not exist on type 'string'.", indexStringValue))
					}
				} else if types.IsAssignable(indexType, types.String) || types.IsAssignable(indexType, types.Symbol) {
					// String or Symbol index - accessing string properties (like Symbol.iterator)
					resultType = types.Any // String properties can have arbitrary types
				} else {
					c.addError(node.Index, fmt.Sprintf("string index must be of type number, string, or symbol, got %s", indexType.String()))
					resultType = types.Any
				}
			} else if isIndexStringLiteral {
				resultType = c.getPropertyTypeFromType(base, indexStringValue, false)
				if resultType == types.Never {
					c.addErrorWithCode(node.Index, errors.TS2339, fmt.Sprintf("Property '%s' does not exist on type '%s'.", indexStringValue, leftType.String()))
				}
			} else {
				c.reportNonIndexable(node, leftType)
			}

		case *types.TypeParameterType:
			if isIndexStringLiteral {
				if base.Parameter != nil && base.Parameter.Constraint != nil {
					resultType = c.getPropertyTypeFromType(base.Parameter.Constraint, indexStringValue, false)
				} else {
					resultType = c.getPropertyTypeFromType(types.NewObjectType(), indexStringValue, false)
					if resultType == types.Never {
						resultType = types.Any
					}
				}
				if resultType == types.Never {
					c.addErrorWithCode(node.Index, errors.TS2339, fmt.Sprintf("Property '%s' does not exist on type '%s'.", indexStringValue, leftType.String()))
				}
			} else if base.Parameter == nil || base.Parameter.Constraint == nil {
				resultType = types.Any
			} else {
				c.reportNonIndexable(node, leftType)
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
							c.reportInvalidIndexType(node, indexType)
							resultType = types.Any
						}
					}
				} else {
					c.reportNonIndexable(node, leftType)
				}
			} else {
				c.reportNonIndexable(node, leftType)
			}

		default:
			c.reportNonIndexable(node, leftType)
		}
	}

	if indexKey := expressionToNarrowingKey(node); indexKey != "" {
		for env := c.env; env != nil; env = env.outer {
			if env.narrowings == nil {
				continue
			}
			if narrowedType, exists := env.narrowings[indexKey]; exists {
				debugPrintf("// [Checker IndexExpr] Using narrowed type for %s: %s\n", indexKey, narrowedType.String())
				resultType = narrowedType
				break
			}
			if complementType, exists := env.narrowings[indexKey+"__complement"]; exists {
				if unionType, ok := resultType.(*types.UnionType); ok {
					if complementUnion, ok := complementType.(*types.UnionType); ok {
						for _, ct := range complementUnion.Types {
							if ut, ok := resultType.(*types.UnionType); ok {
								resultType = ut.RemoveType(ct)
							}
						}
					} else {
						resultType = unionType.RemoveType(complementType)
					}
					debugPrintf("// [Checker IndexExpr] Using complement narrowing for %s: %s\n", indexKey, resultType.String())
				}
				break
			}
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
	widenedObjectType := c.apparentType(types.GetWidenedType(objectType))
	if ro, ok := widenedObjectType.(*types.ReadonlyType); ok {
		widenedObjectType = types.GetWidenedType(ro.InnerType)
	}

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
			} else if methodType := c.env.GetPrimitivePrototypeMethodType("array", propertyName); methodType != nil {
				baseResultType = c.instantiateGenericMethod(methodType, obj.ElementType)
			} else {
				// Array methods should be resolved through the builtins system
				c.reportPropertyNotFound(node.Property, node.Object, propertyName, obj.String())
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
			} else if inherited, ok := obj.GetEffectiveProperties()[propertyName]; ok && inherited != nil {
				baseResultType = inherited
			} else if indexed := stringIndexValueType(obj); indexed != nil && !obj.IsCallable() {
				// `r?.p` over `Record<string, V>` reads V, as `r.p` does.
				baseResultType = indexed
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
		case *types.ObjectTypeMarker:
			// typeof x === "object" narrows to object type marker
			// Property access on object type returns any (same as TypeScript)
			baseResultType = types.Any
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
	} else if baseResultType == types.Any {
		// `any | undefined` is `any`
		resultType = types.Any
	} else {
		// Create union type: baseResultType | undefined
		resultType = &types.UnionType{
			Types: []types.Type{baseResultType, types.Undefined},
		}
	}

	// 6. Set the computed type on the OptionalChainingExpression node itself
	resultType = c.checkOptionalContinuation(node.Continuation, resultType, node.Object)
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
	var baseResultType types.Type

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
	} else if baseResultType == types.Any && widenedObjectType == types.Any {
		resultType = types.Any
	} else {
		resultType = &types.UnionType{
			Types: []types.Type{baseResultType, types.Undefined},
		}
	}

	resultType = c.checkOptionalContinuation(node.Continuation, resultType, nil)
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
	var baseResultType types.Type

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
	} else if baseResultType == types.Any && widenedFunctionType == types.Any {
		resultType = types.Any
	} else {
		resultType = &types.UnionType{
			Types: []types.Type{baseResultType, types.Undefined},
		}
	}

	resultType = c.checkOptionalContinuation(node.Continuation, resultType, nil)
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
	var explicitTypeArgs []types.Type
	if len(node.TypeArguments) > 0 {
		debugPrintf("// [Checker NewExpression] Processing %d type arguments\n", len(node.TypeArguments))
		// Check type arguments - these are type annotations, not expressions
		typeArgs := make([]types.Type, len(node.TypeArguments))
		explicitTypeArgs = typeArgs
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

	c.markContextuallyTypedArguments(node.Arguments, constructorType)

	// Check if trying to instantiate an abstract class
	if ident, ok := node.Constructor.(*parser.Identifier); ok {
		if c.abstractClasses[ident.Value] {
			c.addErrorWithCode(node, errors.TS2511, "Cannot create an instance of an abstract class.")
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
				skipArityCheck := actualArgCount == -1 // -1 signals unknown length arrays in spreads

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

					if !skipArityCheck && actualArgCount < minExpectedArgs {
						c.addError(node, fmt.Sprintf("Constructor expected at least %d arguments but got %d.", minExpectedArgs, actualArgCount))
					} else {
						// Check fixed arguments
						fixedArgsOk := c.checkFixedArgumentsWithSpread(node.Arguments, constructorSig.ParameterTypes, constructorSig.IsVariadic, constructorSig.OptionalParams)

						// Check variadic arguments
						if fixedArgsOk && constructorSig.RestParameterType != nil {
							arrayType, isArray := constructorSig.RestParameterType.(*types.ArrayType)
							if !isArray {
								c.visitRemainingArguments(node.Arguments, len(constructorSig.ParameterTypes))
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

										if c.isSpreadableIterableType(argType) {
											spreadElementType := c.getSpreadElementType(argType)
											if !types.IsAssignable(spreadElementType, variadicElementType) {
												c.addError(spreadElement, fmt.Sprintf("spread element: cannot assign type '%s' to parameter element type '%s'", spreadElementType.String(), variadicElementType.String()))
											}
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

					if !skipArityCheck && actualArgCount < minRequiredArgs {
						c.addErrorWithCode(node, errors.TS2554, formatArityError(minRequiredArgs, expectedArgCount, actualArgCount))
					} else if !skipArityCheck && actualArgCount > expectedArgCount && expectedArgCount > 0 {
						// Only enforce max args if constructor has declared parameters
						// (allow extra args for constructors with no params - they may use 'arguments')
						c.addErrorWithCode(node, errors.TS2554, formatArityError(minRequiredArgs, expectedArgCount, actualArgCount))
					} else {
						c.checkFixedArgumentsWithSpread(node.Arguments, constructorSig.ParameterTypes, constructorSig.IsVariadic, constructorSig.OptionalParams)
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
	if inst := c.builtinGenericCtorResult(constructorType, explicitTypeArgs, node.Arguments); inst != nil {
		resultType = inst
	} else if objType, ok := constructorType.(*types.ObjectType); ok {
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
		// Invalid constructor type. Only types we can judge are reported:
		// intersections of constructor types (mixins) and other deferred types
		// are given the benefit of the doubt.
		if c.isDefinitelyNotConstructor(constructorType) {
			c.addErrorWithCode(leftmostExpression(node.Constructor), errors.TS2351, "This expression is not constructable.")
		}
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
		// Special handling: 'arguments' is available in function scope but may not be in env yet
		_, _, found := c.env.Resolve(ident.Value)
		if !found && ident.Value != "arguments" {
			// Identifier doesn't exist (and it's not 'arguments') - typeof will return "undefined" string literal type
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

	if node.Token.Literal == "as" {
		c.checkDuplicateTypeAssertionProperties(node.TargetType)
	}

	if isConstAssertionTarget(node.TargetType) {
		node.SetComputedType(constAssertionType(node.Expression))
		return
	}

	// Resolve the target type
	targetType := c.resolveTypeAnnotation(node.TargetType)
	if targetType == nil {
		// resolveTypeAnnotation has already reported why the target is unusable.
		node.SetComputedType(types.Any)
		return
	}

	// Validate the type assertion according to TypeScript rules
	if !c.isValidTypeAssertion(sourceType, targetType) {
		var errNode parser.Node = node
		if node.Token.Literal == "as" && node.Expression != nil {
			errNode = node.Expression
		}
		c.addErrorAtStart(errNode, errors.TS2352, fmt.Sprintf("Conversion of type '%s' to type '%s' may be a mistake because neither type sufficiently overlaps with the other. If this was intentional, convert the expression to 'unknown' first.",
			sourceType.String(), targetType.String()))
	}

	// The result type is always the target type
	node.SetComputedType(targetType)
}

func (c *Checker) checkDuplicateTypeAssertionProperties(targetType parser.Expression) {
	objectType, ok := targetType.(*parser.ObjectTypeExpression)
	if !ok {
		return
	}

	seen := make(map[string]parser.Expression)
	seenName := make(map[string]*parser.Identifier)
	reported := make(map[string]bool)
	for _, prop := range objectType.Properties {
		if prop.Name == nil || prop.IsCallSignature || prop.IsConstructSignature || prop.IsIndexSignature || prop.IsComputedProperty {
			continue
		}

		if previousType, exists := seen[prop.Name.Value]; exists {
			if _, previousIsFunction := previousType.(*parser.FunctionTypeExpression); previousIsFunction {
				if _, currentIsFunction := prop.Type.(*parser.FunctionTypeExpression); currentIsFunction {
					continue
				}
			}
			// TypeScript flags every occurrence of the duplicated identifier,
			// including the first one (only once).
			if !reported[prop.Name.Value] {
				c.addErrorWithCode(seenName[prop.Name.Value], errors.TS2300, fmt.Sprintf("Duplicate identifier '%s'.", prop.Name.Value))
				reported[prop.Name.Value] = true
			}
			c.addErrorWithCode(prop.Name, errors.TS2300, fmt.Sprintf("Duplicate identifier '%s'.", prop.Name.Value))
			continue
		}

		seen[prop.Name.Value] = prop.Type
		seenName[prop.Name.Value] = prop.Name
	}
}

// checkSatisfiesExpression handles satisfies expressions (value satisfies Type)
func (c *Checker) checkSatisfiesExpression(node *parser.SatisfiesExpression) {
	// Resolve the target type first: it is the contextual type of the expression
	targetType := c.resolveTypeAnnotation(node.TargetType)
	if targetType == nil {
		c.visit(node.Expression)
		c.addError(node.TargetType, "invalid type in satisfies expression")
		node.SetComputedType(types.Any)
		return
	}

	// Visit the expression being validated, contextually typed by the target
	c.visitWithContext(node.Expression, &ContextualType{ExpectedType: targetType, IsContextual: true})
	sourceType := node.Expression.GetComputedType()
	if sourceType == nil {
		sourceType = types.Any
	}

	// `e satisfies T` relates the (fresh) expression type to T: elaboration
	// reports on the offending property or element, excess properties are
	// TS2353, and otherwise the head message is TS1360.
	if !c.assignableToFresh(node.Expression, sourceType, targetType) {
		c.reportNotAssignable(node.Expression, node.Expression, sourceType, targetType, headSatisfies)
	}

	// The result type is the ORIGINAL expression type, NOT the target type
	// This is the key difference from type assertions
	node.SetComputedType(sourceType)
}

// checkNonNullExpression handles non-null assertion expressions (x!)
// This removes null and undefined from the type
func (c *Checker) checkNonNullExpression(node *parser.NonNullExpression) {
	// Visit the expression being asserted
	c.visit(node.Expression)
	sourceType := node.Expression.GetComputedType()
	if sourceType == nil {
		sourceType = types.Any
	}

	// Remove null and undefined from the type
	resultType := types.RemoveNullUndefined(sourceType)
	node.SetComputedType(resultType)
}

// checkObjectLiteralSatisfies performs strict checking for object literals in satisfies expressions
func (c *Checker) checkObjectLiteralSatisfies(objectLit *parser.ObjectLiteral, targetType types.Type, satisfiesNode *parser.SatisfiesExpression) {
	sourceType := objectLit.GetComputedType()
	if sourceType == nil {
		return
	}

	// First check if the source type is assignable to the target type
	if !types.IsAssignable(sourceType, targetType) {
		c.addErrorWithCode(satisfiesNode.Expression, errors.TS1360, fmt.Sprintf("Type '%s' does not satisfy the expected type '%s'.",
			types.GetWidenedType(sourceType).String(), targetType.String()))
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

	// checkAssertionDeferred: with the source's literal types replaced by
	// their base types, the assertion is fine when the target is comparable
	// to the source or the source is comparable to the target.
	sourceBase := baseTypeOfLiterals(sourceType)
	sourceBase = c.resolveStructural(sourceBase)
	targetType = c.resolveStructural(targetType)
	if types.IsComparable(targetType, sourceBase) || types.IsComparable(sourceBase, targetType) {
		return true
	}
	return false
}

// baseTypeOfLiterals replaces literal types (including those inside unions)
// by their primitive base types.
func baseTypeOfLiterals(t types.Type) types.Type {
	if u, ok := t.(*types.UnionType); ok {
		members := make([]types.Type, len(u.Types))
		for i, m := range u.Types {
			members[i] = baseTypeOfLiterals(m)
		}
		return types.NewUnionType(members...)
	}
	return types.GetWidenedType(t)
}

// isPrimitiveType checks if a type is a primitive type
func (c *Checker) isPrimitiveType(t types.Type) bool {
	return t == types.String || t == types.Number || t == types.Boolean ||
		t == types.Null || t == types.Undefined
}

// checkInOperator handles type checking for the 'in' operator ("prop" in obj)
func (c *Checker) checkInOperator(leftType, rightType types.Type, node *parser.InfixExpression) {
	// Left operand (property name) should be string, number, or symbol (per ECMAScript spec)
	if leftType != types.Any && leftType != types.String && leftType != types.Number && leftType != types.Symbol {
		// Check if it's a literal string or number type
		if !c.isStringOrNumberLiteralType(leftType) {
			c.addError(node.Left, fmt.Sprintf("the left-hand side of 'in' must be of type 'string', 'number', or 'symbol', but got '%s'", leftType.String()))
		}
	}

	// Right operand (object) should be an object type
	if rightType != types.Any && !c.isObjectType(c.apparentType(rightType)) {
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
		// RegExp is a real ordinary object at runtime, but the checker
		// models it as a distinct *types.Primitive marker (see
		// pkg/types/primitive.go) rather than an *types.ObjectType, so it
		// needs its own case here - otherwise `"test" in /x/` was rejected
		// at check time before the 'in' operator's own object-ness fix in
		// the VM (OpIn, pkg/vm/vm.go) ever ran.
		return t == types.Any || t == types.RegExp || t == types.NonPrimitive
	}
}

// checkInstanceofOperator checks the instanceof operator usage
func (c *Checker) checkInstanceofOperator(leftType, rightType types.Type, node *parser.InfixExpression) {
	// TS2358: the left operand must be `any`, an object type or a type
	// parameter - a primitive can never be an instance of anything.
	if leftType != nil && isPrimitiveOperandType(leftType) {
		c.addErrorWithCode(node.Left, errors.TS2358, "The left-hand side of an 'instanceof' expression must be of type 'any', an object type or a type parameter.")
	}

	// Right operand must be a constructor function
	if rightType != types.Any && c.isDefinitelyNotConstructor(rightType) {
		c.addErrorWithCode(node.Right, errors.TS2359, "The right-hand side of an 'instanceof' expression must be either of type 'any', a class, function, or other type assignable to the 'Function' interface type, or an object type with a 'Symbol.hasInstance' method.")
	}
}

// isConstructorType checks if a type represents a constructor function
// isDefinitelyNotConstructor reports whether t certainly has no call or
// construct signature and cannot be a Function subtype: primitives, literals
// and plain object types. Anything we cannot judge answers false.
func (c *Checker) isDefinitelyNotConstructor(t types.Type) bool {
	switch tt := types.GetEffectiveType(t).(type) {
	case *types.Primitive:
		switch tt {
		case types.String, types.Number, types.Boolean, types.BigInt, types.Symbol, types.Null, types.Undefined, types.Void:
			return true
		}
		return false
	case *types.LiteralType:
		return true
	case *types.ObjectType:
		return !tt.IsCallable() && len(tt.ConstructSignatures) == 0 && len(tt.BaseTypes) == 0 && tt.ClassMeta == nil
	case *types.UnionType:
		for _, member := range tt.Types {
			if !c.isDefinitelyNotConstructor(member) {
				return false
			}
		}
		return true
	}
	return false
}

func (c *Checker) isConstructorType(t types.Type) bool {
	if objType, ok := t.(*types.ObjectType); ok {
		return objType.IsCallable() || len(objType.ConstructSignatures) > 0
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
	if c.noImplicitAny && node.Value != nil {
		c.markFunctionsUnder(node.Value, 0) // typed by the generator's contextual yield type
	}
	// Outside a generator body the parser has already reported TS1163. tsc's
	// checkYieldExpression then returns any without checking the operand; we
	// still check it (an unknown name there is a real bug) unless in tsc mode.
	if !c.inGeneratorFunction && !c.beyondTsc() {
		node.SetComputedType(types.Any)
		return
	}

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
			// Per TypeScript: any is always accepted for yield*
			if valueType == types.Any {
				yieldedType = types.Any
				goto setYieldedType
			}

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

// isSpreadableIterableType returns true if a type has a usable Symbol.iterator
// shape for spread syntax. A bare computed property is not enough because
// classes can define [Symbol.iterator]() without returning an iterator.
func (c *Checker) isSpreadableIterableType(t types.Type) bool {
	if t == nil {
		return false
	}
	if genericType, ok := t.(*types.GenericType); ok {
		if genericType.Name == "Iterable" || genericType.Name == "Iterator" || genericType.Name == "Generator" {
			return true
		}
	}
	if genericRef, ok := t.(*types.GenericTypeAliasForwardReference); ok {
		if genericRef.AliasName == "Iterable" || genericRef.AliasName == "Iterator" || genericRef.AliasName == "Generator" {
			return true
		}
	}
	if parameterizedRef, ok := t.(*types.ParameterizedForwardReferenceType); ok {
		if parameterizedRef.ClassName == "Iterable" || parameterizedRef.ClassName == "Iterator" || parameterizedRef.ClassName == "Generator" {
			return true
		}
	}
	if instantiatedType, ok := t.(*types.InstantiatedType); ok {
		if instantiatedType.Generic != nil && (instantiatedType.Generic.Name == "Iterable" || instantiatedType.Generic.Name == "Iterator" || instantiatedType.Generic.Name == "Generator") {
			return true
		}
	}
	methodType := c.getPropertyTypeFromType(t, "__COMPUTED_PROPERTY__", false)
	if methodType == nil || methodType == types.Never {
		return false
	}

	if c.hasIteratorNext(t) {
		return true
	}

	if methodReturn := callableReturnType(methodType); methodReturn != nil && methodReturn != types.Any {
		return c.hasIteratorNext(methodReturn)
	}

	return false
}

func (c *Checker) hasIteratorNext(t types.Type) bool {
	nextType := c.getPropertyTypeFromType(t, "next", false)
	return nextType != nil && nextType != types.Never
}

func (c *Checker) getSpreadElementType(t types.Type) types.Type {
	if t == nil {
		return types.Any
	}
	if arrayType, ok := t.(*types.ArrayType); ok {
		if arrayType.ElementType != nil {
			return arrayType.ElementType
		}
		return types.Any
	}
	if tupleType, ok := t.(*types.TupleType); ok {
		return getTupleElementUnion(tupleType)
	}
	if instantiatedType, ok := t.(*types.InstantiatedType); ok {
		if instantiatedType.Generic != nil && len(instantiatedType.TypeArguments) > 0 &&
			(instantiatedType.Generic.Name == "Iterable" || instantiatedType.Generic.Name == "Iterator" || instantiatedType.Generic.Name == "Generator") {
			return instantiatedType.TypeArguments[0]
		}
	}

	// If t isn't already iterator-shaped (no .next of its own), it's an iterable:
	// resolve its [Symbol.iterator]() return type and look for .next on that instead.
	iteratorType := t
	if !c.hasIteratorNext(t) {
		if methodType := c.getPropertyTypeFromType(t, "__COMPUTED_PROPERTY__", false); methodType != nil && methodType != types.Never {
			if methodReturn := callableReturnType(methodType); methodReturn != nil && methodReturn != types.Any {
				iteratorType = methodReturn
			}
		}
	}

	nextType := c.getPropertyTypeFromType(iteratorType, "next", false)
	nextReturnType := callableReturnType(nextType)
	if nextReturnType == nil || nextReturnType == types.Any || nextReturnType == types.Never {
		return types.Any
	}
	valueType := c.getPropertyTypeFromType(nextReturnType, "value", false)
	if valueType == nil || valueType == types.Never {
		return types.Any
	}
	return valueType
}

func callableReturnType(t types.Type) types.Type {
	objType, ok := t.(*types.ObjectType)
	if !ok || len(objType.CallSignatures) == 0 {
		return nil
	}
	return objType.CallSignatures[0].ReturnType
}

// getTupleElementUnion creates a union type from all element types in a tuple
// Used when indexing with a general number (not a literal)
func getTupleElementUnion(tuple *types.TupleType) types.Type {
	if len(tuple.ElementTypes) == 0 && tuple.RestElementType == nil {
		return types.Never
	}

	var allTypes []types.Type
	for _, elemType := range tuple.ElementTypes {
		if elemType != nil {
			allTypes = append(allTypes, elemType)
		}
	}

	// Include rest element type if present
	if tuple.RestElementType != nil {
		allTypes = append(allTypes, tuple.RestElementType)
	}

	if len(allTypes) == 0 {
		return types.Never
	}
	if len(allTypes) == 1 {
		return allTypes[0]
	}
	return types.NewUnionType(allTypes...)
}

// classHasExtendsClause reports whether the named class (or something we cannot
// look up) may inherit members, so missing statics should not be diagnosed.
func (c *Checker) classHasExtendsClause(className string) bool {
	instance := c.getClassInstanceType(className)
	return instance == nil || instance.ClassMeta == nil || instance.ClassMeta.HasExtendsClause
}

// checkOptionalContinuation types the rest of an optional chain (`.c`, `[i]`,
// `(args)` after `a?.b`) against the non-nullish type of its head and returns
// the type of the whole chain. The continuation's root receiver is nil in the
// tree; a ChainBase carrying the head type stands in for it while it is typed.
func (c *Checker) checkOptionalContinuation(cont parser.Expression, head types.Type, receiver parser.Expression) types.Type {
	if cont == nil {
		return head
	}
	nonNullish := types.RemoveNullishTypes(head)
	if nonNullish == nil || nonNullish == types.Never {
		return types.Undefined
	}

	base := &parser.ChainBase{Token: parser.GetTokenFromNode(cont), Receiver: receiver}
	base.SetComputedType(nonNullish)

	var restore func()
	// Find the innermost link, whose receiver slot is nil.
	cur := cont
	for {
		switch n := cur.(type) {
		case *parser.MemberExpression:
			if n.Object == nil {
				n.Object = base
				restore = func() { n.Object = nil }
			} else {
				cur = n.Object
				continue
			}
		case *parser.IndexExpression:
			if n.Left == nil {
				n.Left = base
				restore = func() { n.Left = nil }
			} else {
				cur = n.Left
				continue
			}
		case *parser.CallExpression:
			if n.Function == nil {
				n.Function = base
				restore = func() { n.Function = nil }
			} else {
				cur = n.Function
				continue
			}
		default:
			return head
		}
		break
	}
	defer restore()

	c.visit(cont)
	result := cont.GetComputedType()
	if result == nil || result == types.Any {
		return types.Any
	}
	return types.NewUnionType(result, types.Undefined)
}

// builtinGenericCtorResult types `new Map<K, V>()` and its siblings. Built-in
// collection constructors are declared as callable (not constructable) types
// whose return type is the generic instance type, so the construct result is
// that type instantiated with the written type arguments; type parameters
// without an argument are any. It returns nil for any other constructor.
func (c *Checker) builtinGenericCtorResult(ctor types.Type, typeArgs []types.Type, callArgs []parser.Expression) types.Type {
	obj, ok := ctor.(*types.ObjectType)
	if g, isGeneric := ctor.(*types.GenericType); isGeneric {
		obj, ok = g.Body.(*types.ObjectType)
	}
	if ok && obj != nil && len(obj.ConstructSignatures) == 0 && len(obj.CallSignatures) > 0 && len(typeArgs) == 1 {
		// new Array<T>() is T[].
		if arr, isArr := obj.CallSignatures[0].ReturnType.(*types.ArrayType); isArr && arr.ElementType == types.Any {
			return &types.ArrayType{ElementType: typeArgs[0]}
		}
	}
	if !ok || obj == nil || len(obj.ConstructSignatures) > 0 || len(obj.CallSignatures) != 1 {
		return nil
	}
	instance, ok := obj.CallSignatures[0].ReturnType.(*types.GenericType)
	if !ok || len(instance.TypeParameters) == 0 {
		return nil
	}
	args := make([]types.Type, len(instance.TypeParameters))
	var inferred []types.Type
	if len(typeArgs) == 0 && (instance.Name == "Map" || instance.Name == "Set") {
		inferred = inferCollectionTypeArgs(len(args), callArgs)
	}
	for i := range args {
		switch {
		case i < len(typeArgs):
			args[i] = typeArgs[i]
		case i < len(inferred) && inferred[i] != nil:
			args[i] = inferred[i]
		default:
			args[i] = types.Any
		}
	}
	return c.instantiateGenericType(instance, args, nil)
}

// stringIndexValueType is the value type of obj's string (or any) index
// signature, or nil when it has none.
func stringIndexValueType(obj *types.ObjectType) types.Type {
	for _, sig := range obj.IndexSignatures {
		if sig.KeyType == types.String || sig.KeyType == types.Any {
			if sig.ValueType == nil {
				return types.Any
			}
			return sig.ValueType
		}
	}
	return nil
}

// inferCollectionTypeArgs infers the type arguments of `new Map(entries)` and
// `new Set(items)` from the iterable written as the argument: the union of the
// (widened) key and value types of the entries, or of the items. It returns nil
// when the argument says nothing (`new Map()`, a non-literal iterable).
func inferCollectionTypeArgs(paramCount int, callArgs []parser.Expression) []types.Type {
	if len(callArgs) == 0 {
		return nil
	}
	widen := func(e parser.Expression) types.Type {
		t := e.GetComputedType()
		if t == nil {
			return nil
		}
		return types.DeeplyWidenType(t)
	}
	union := func(ts []types.Type) types.Type {
		if len(ts) == 0 {
			return nil
		}
		return types.NewUnionType(ts...)
	}
	lit, isLit := callArgs[0].(*parser.ArrayLiteral)
	switch paramCount {
	case 1:
		if isLit {
			var items []types.Type
			for _, el := range lit.Elements {
				if _, spread := el.(*parser.SpreadElement); spread {
					return nil
				}
				if t := widen(el); t != nil {
					items = append(items, t)
				}
			}
			return []types.Type{union(items)}
		}
		if arr, ok := callArgs[0].GetComputedType().(*types.ArrayType); ok {
			return []types.Type{arr.ElementType}
		}
	case 2:
		if !isLit {
			return nil
		}
		var keys, values []types.Type
		for _, el := range lit.Elements {
			pair, ok := el.(*parser.ArrayLiteral)
			if !ok || len(pair.Elements) != 2 {
				return nil
			}
			if k := widen(pair.Elements[0]); k != nil {
				keys = append(keys, k)
			}
			if v := widen(pair.Elements[1]); v != nil {
				values = append(values, v)
			}
		}
		return []types.Type{union(keys), union(values)}
	}
	return nil
}

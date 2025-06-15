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

		// --- Use deeplyWidenType on each element ---
		generalizedType := types.DeeplyWidenType(elemType)
		// --- End Deep Widen ---

		generalizedElementTypes = append(generalizedElementTypes, generalizedType)
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

		// For tuple context, we need the array literal to have exactly the right number of elements
		if len(node.Elements) != len(tupleType.ElementTypes) {
			// If length doesn't match, fall back to regular array literal checking
			debugPrintf("// [Checker ArrayLitContext] Element count mismatch: expected %d, got %d. Using regular array checking.\n", len(tupleType.ElementTypes), len(node.Elements))
			c.checkArrayLiteral(node)
			return
		}

		// Check each element against the corresponding tuple element type
		elementTypesMatch := true
		for i, elemNode := range node.Elements {
			expectedElemType := tupleType.ElementTypes[i]
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
			c.visitWithContext(elemNode, &ContextualType{
				ExpectedType: arrayType.ElementType,
				IsContextual: true,
			})
		}

		// Use the expected array type as the result
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
		default:
			c.addError(prop.Key, "object key must be an identifier, string, or number literal")
			continue // Skip this property if key type is invalid
		}

		// Check for duplicate keys
		if seenKeys[keyName] {
			c.addError(prop.Key, fmt.Sprintf("duplicate property key: '%s'", keyName))
			// Continue checking value type anyway? Yes, to catch more errors.
		}
		seenKeys[keyName] = true

		// For non-function properties, visit and store the type immediately
		if _, isFunctionLiteral := prop.Value.(*parser.FunctionLiteral); !isFunctionLiteral {
			if _, isArrowFunction := prop.Value.(*parser.ArrowFunctionLiteral); !isArrowFunction {
				if _, isShorthandMethod := prop.Value.(*parser.ShorthandMethod); !isShorthandMethod {
					// Visit the value to determine its type
					c.visit(prop.Value)
					valueType := prop.Value.GetComputedType()
					if valueType == nil {
						// If visiting the value failed, checker should have added error.
						// Default to Any to prevent cascading nil errors.
						valueType = types.Any
					}
					fields[keyName] = valueType
					preliminaryObjType.Properties[keyName] = valueType
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
		default:
			continue // Already handled error in first pass
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
		default:
			continue // Already handled error in first pass
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
		}
	}

	// Restore outer this context
	c.currentThisType = outerThisType
	debugPrintf("// [Checker ObjectLit] Restored this context to: %v\n", outerThisType)

	// Create the final ObjectType
	objType := &types.ObjectType{Properties: fields}

	// Set the computed type for the ObjectLiteral node itself
	node.SetComputedType(objType)
	debugPrintf("// [Checker ObjectLit] Computed type: %s\n", objType.String())
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

// Helper function
func (c *Checker) checkMemberExpression(node *parser.MemberExpression) {
	// 1. Visit the object part
	c.visit(node.Object)
	objectType := node.Object.GetComputedType()
	if objectType == nil {
		// If visiting the object failed, checker should have added error.
		// Set objectType to Any to prevent cascading nil errors here.
		objectType = types.Any
	}

	// 2. Get the property name (Property is always an Identifier in MemberExpression)
	propertyName := node.Property.Value // node.Property is *parser.Identifier

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
	} else {
		// Use a type switch for struct-based types
		switch obj := widenedObjectType.(type) {
		case *types.ArrayType:
			if propertyName == "length" {
				resultType = types.Number // Array.length is number
			} else {
				// Check prototype registry for Array methods
				if methodType := c.env.GetPrimitivePrototypeMethodType("array", propertyName); methodType != nil {
					resultType = methodType
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
				// Look for the property in the object's fields
				fieldType, exists := obj.Properties[propertyName]
				if exists {
					// Property found
					if fieldType == nil { // Should ideally not happen if checker populates correctly
						c.addError(node.Property, fmt.Sprintf("internal checker error: property '%s' has nil type in ObjectType", propertyName))
						resultType = types.Never
					} else {
						resultType = fieldType
					}
				} else if obj.IsCallable() {
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
		case *types.IntersectionType:
			// Handle property access on intersection types
			propType := c.getPropertyTypeFromIntersection(obj, propertyName)
			if propType == types.Never {
				c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on intersection type %s", propertyName, obj.String()))
			}
			resultType = propType
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

func (c *Checker) checkIndexExpression(node *parser.IndexExpression) {
	// 1. Visit the base expression (array/object)
	c.visit(node.Left)
	leftType := node.Left.GetComputedType()

	// 2. Visit the index expression
	c.visit(node.Index)
	indexType := node.Index.GetComputedType()

	var resultType types.Type = types.Any // Default result type on error

	// 3. Check base type (allow Array for now)
	// First handle the special case of 'any'
	if leftType == types.Any {
		// Indexing into 'any' always returns 'any' - this is standard TypeScript behavior
		resultType = types.Any
	} else {
		switch base := leftType.(type) {
		case *types.ArrayType:
			// Base is ArrayType
			// 4. Check index type (must be number for array)
			if !types.IsAssignable(indexType, types.Number) {
				c.addError(node.Index, fmt.Sprintf("array index must be of type number, got %s", indexType.String()))
				// Proceed with Any as result type
			} else {
				// Index type is valid, result type is the array's element type
				if base.ElementType != nil {
					resultType = base.ElementType
				} else {
					resultType = types.Unknown
				}
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

			if widenedIndexType == types.String || widenedIndexType == types.Number || widenedIndexType == types.Any {
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
						c.addError(node.Index, fmt.Sprintf("property '%s' does not exist on type %s", indexStringValue, base.String()))
						// resultType remains Error
					}
				} else {
					// Index is a general string/number/any - cannot determine specific property type.
					// TODO: Support index signatures later?
					// For now, result is 'any' as we don't know which property is accessed.
					resultType = types.Any
				}
			} else {
				// Invalid index type for object
				c.addError(node.Index, fmt.Sprintf("object index must be of type 'string', 'number', or 'any', got '%s'", indexType.String()))
				// resultType remains Error
			}

		case *types.Primitive:
			// Allow indexing on strings?
			if base == types.String {
				// 4. Check index type (must be number for string)
				if !types.IsAssignable(indexType, types.Number) {
					c.addError(node.Index, fmt.Sprintf("string index must be of type number, got %s", indexType.String()))
				}
				// Result of indexing a string is always a string (or potentially undefined)
				// Let's use string | undefined? Or just string?
				// For simplicity now, let's say string.
				// A more precise type might be: types.NewUnionType(types.String, types.Undefined)
				resultType = types.String
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

	// 2. Get the property name (Property is always an Identifier in OptionalChainingExpression)
	propertyName := node.Property.Value // node.Property is *parser.Identifier

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

func (c *Checker) checkNewExpression(node *parser.NewExpression) {
	// Check the constructor expression
	c.visit(node.Constructor)
	constructorType := node.Constructor.GetComputedType()
	if constructorType == nil {
		constructorType = types.Any
	}

	// Check arguments
	for _, arg := range node.Arguments {
		c.visit(arg)
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
	} else if constructorType == types.Any {
		// If constructor type is Any, result is also Any
		resultType = types.Any
	} else {
		// Invalid constructor type
		c.addError(node.Constructor, fmt.Sprintf("'%s' is not a constructor", constructorType.String()))
		resultType = types.Any
	}

	node.SetComputedType(resultType)
}

// checkTypeofExpression checks the type of a typeof expression.
// This handles TypeScript's typeof operator, which returns a string literal
// like "string", "number", "boolean", "undefined", "object", or "function".
func (c *Checker) checkTypeofExpression(node *parser.TypeofExpression) {
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
	switch t.(type) {
	case *types.ObjectType, *types.ArrayType:
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

package checker

import (
	"fmt"
	"paserati/pkg/builtins"
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
			preliminaryFuncType := c.resolveFunctionLiteralType(funcLit, c.env)
			if preliminaryFuncType == nil {
				preliminaryFuncType = &types.FunctionType{
					ParameterTypes: make([]types.Type, len(funcLit.Parameters)),
					ReturnType:     types.Any,
				}
				for i := range preliminaryFuncType.ParameterTypes {
					preliminaryFuncType.ParameterTypes[i] = types.Any
				}
			}
			preliminaryObjType.Properties[keyName] = preliminaryFuncType
		} else if _, isArrowFunction := prop.Value.(*parser.ArrowFunctionLiteral); isArrowFunction {
			// For arrow functions, we can use a generic function type temporarily
			preliminaryObjType.Properties[keyName] = &types.FunctionType{
				ParameterTypes: []types.Type{}, // We'll refine this later
				ReturnType:     types.Any,
			}
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

			preliminaryFuncType := &types.FunctionType{
				ParameterTypes: paramTypes,
				ReturnType:     returnType,
			}
			preliminaryObjType.Properties[keyName] = preliminaryFuncType
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
			if methodType := builtins.GetPrototypeMethodType("string", propertyName); methodType != nil {
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
				if methodType := builtins.GetPrototypeMethodType("array", propertyName); methodType != nil {
					resultType = methodType
				} else {
					c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on type %s", propertyName, obj.String()))
					// resultType remains types.Never
				}
			}
		case *types.ObjectType: // <<< MODIFIED CASE
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
			} else {
				// Property not found
				c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on type %s", propertyName, obj.String()))
				// resultType remains types.Never
			}
		case *types.FunctionType:
			// Regular function types don't have properties
			c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on function type", propertyName))
			// resultType remains types.Never
		case *types.CallableType:
			// Handle property access on callable types (like String.fromCharCode)
			if propType, exists := obj.Properties[propertyName]; exists {
				resultType = propType
			} else {
				c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on callable type", propertyName))
				// resultType remains types.Never
			}
		case *types.IntersectionType:
			// Handle property access on intersection types
			propType := c.getPropertyTypeFromIntersection(obj, propertyName)
			if propType == types.Never {
				c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on intersection type %s", propertyName, obj.String()))
			}
			resultType = propType
		// Add cases for other struct-based types here if needed (e.g., FunctionType methods?)
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
		} else if propertyName == "charCodeAt" {
			baseResultType = &types.FunctionType{
				ParameterTypes: []types.Type{types.Number},
				ReturnType:     types.Number,
				IsVariadic:     false,
			}
		} else if propertyName == "charAt" {
			baseResultType = &types.FunctionType{
				ParameterTypes: []types.Type{types.Number},
				ReturnType:     types.String,
				IsVariadic:     false,
			}
		} else {
			c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on type 'string'", propertyName))
			// baseResultType remains types.Never
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
			} else if propertyName == "concat" {
				baseResultType = &types.FunctionType{
					ParameterTypes:    []types.Type{}, // No fixed parameters
					ReturnType:        &types.ArrayType{ElementType: types.Any},
					IsVariadic:        true,
					RestParameterType: &types.ArrayType{ElementType: types.Any}, // Accept any values
				}
			} else if propertyName == "push" {
				baseResultType = &types.FunctionType{
					ParameterTypes:    []types.Type{}, // No fixed parameters
					ReturnType:        types.Number,   // Returns new length
					IsVariadic:        true,
					RestParameterType: &types.ArrayType{ElementType: types.Any}, // Accept any values
				}
			} else if propertyName == "pop" {
				baseResultType = &types.FunctionType{
					ParameterTypes: []types.Type{},
					ReturnType:     types.Any,
					IsVariadic:     false,
				}
			} else if propertyName == "join" {
				baseResultType = &types.FunctionType{
					ParameterTypes: []types.Type{types.String}, // Optional separator parameter
					ReturnType:     types.String,               // Returns string
					IsVariadic:     false,
					OptionalParams: []bool{true}, // Separator is optional
				}
			} else {
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
			} else {
				// Property not found - for optional chaining, this is OK, just return undefined
				// Don't add an error like regular member access would
				baseResultType = types.Undefined
			}
		case *types.FunctionType:
			// Handle static properties/methods on function types (like String.fromCharCode)
			// Check if the object is a builtin constructor that has static methods
			if objIdentifier, ok := node.Object.(*parser.Identifier); ok {
				// Look for builtin static method: ConstructorName.methodName
				staticMethodName := objIdentifier.Value + "." + propertyName
				staticMethodType := c.getBuiltinType(staticMethodName)
				if staticMethodType != nil {
					baseResultType = staticMethodType
				} else {
					c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on function type", propertyName))
					// baseResultType remains types.Never
				}
			} else {
				c.addError(node.Property, fmt.Sprintf("property '%s' does not exist on function type", propertyName))
				// baseResultType remains types.Never
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

	// Determine the result type based on the constructor type
	var resultType types.Type
	if constructorTypeVal, ok := constructorType.(*types.ConstructorType); ok {
		// Direct constructor type
		if len(node.Arguments) != len(constructorTypeVal.ParameterTypes) {
			c.addError(node, fmt.Sprintf("constructor expects %d arguments but got %d",
				len(constructorTypeVal.ParameterTypes), len(node.Arguments)))
		} else {
			// Check argument types
			for i, arg := range node.Arguments {
				argType := arg.GetComputedType()
				if argType == nil {
					argType = types.Any
				}
				expectedType := constructorTypeVal.ParameterTypes[i]
				if !types.IsAssignable(argType, expectedType) {
					c.addError(arg, fmt.Sprintf("argument %d: cannot assign %s to %s",
						i+1, argType.String(), expectedType.String()))
				}
			}
		}
		resultType = constructorTypeVal.ConstructedType
	} else if objType, ok := constructorType.(*types.ObjectType); ok {
		// Check if this object type has a constructor signature ("new" property)
		if newProp, hasNew := objType.Properties["new"]; hasNew {
			if ctorType, isConstructor := newProp.(*types.ConstructorType); isConstructor {
				// Validate arguments against constructor signature
				if len(node.Arguments) != len(ctorType.ParameterTypes) {
					c.addError(node, fmt.Sprintf("constructor expects %d arguments but got %d",
						len(ctorType.ParameterTypes), len(node.Arguments)))
				} else {
					// Check argument types
					for i, arg := range node.Arguments {
						argType := arg.GetComputedType()
						if argType == nil {
							argType = types.Any
						}
						expectedType := ctorType.ParameterTypes[i]
						if !types.IsAssignable(argType, expectedType) {
							c.addError(arg, fmt.Sprintf("argument %d: cannot assign %s to %s",
								i+1, argType.String(), expectedType.String()))
						}
					}
				}
				resultType = ctorType.ConstructedType
			} else {
				c.addError(node.Constructor, fmt.Sprintf("'new' property is not a constructor type"))
				resultType = types.Any
			}
		} else {
			c.addError(node.Constructor, fmt.Sprintf("object type does not have a constructor signature"))
			resultType = types.Any
		}
	} else if _, ok := constructorType.(*types.FunctionType); ok {
		// For function constructors, the result is typically an object
		// In a more sophisticated implementation, we'd track constructor return types
		// For now, we'll assume constructors return objects
		resultType = &types.ObjectType{Properties: make(map[string]types.Type)}
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

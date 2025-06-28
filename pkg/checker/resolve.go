package checker

import (
	"fmt"
	"paserati/pkg/parser"
	"paserati/pkg/types"
	"paserati/pkg/vm"
	"strings"
)

// --- Helper Functions ---

// resolveTypeAnnotation converts a parser node representing a type annotation
// into a types.Type representation.
func (c *Checker) resolveTypeAnnotation(node parser.Expression) types.Type {
	if node == nil {
		// No annotation provided, perhaps default to any or handle elsewhere?
		// For now, returning nil might be okay, caller decides default.
		return nil
	}

	// Dispatch based on the structure of the type expression node
	switch node := node.(type) {
	case *parser.Identifier:
		// --- ADDED: Check if node or node.Value is nil ---
		if node == nil || node.Value == "" {
			debugPrintf("// [Checker resolveTypeAnno Ident] ERROR: Node (%p) or Node.Value is nil/empty!\n", node)
			return nil // Return nil early if node is bad
		}
		debugPrintf("// [Checker resolveTypeAnno Ident] Processing identifier: '%s'\n", node.Value)
		// --- END ADDED ---

		// --- NEW: Check if this is a type parameter first ---
		if typeParam, found := c.env.ResolveTypeParameter(node.Value); found {
			debugPrintf("// [Checker resolveTypeAnno Ident] Resolved '%s' as type parameter\n", node.Value)
			return &types.TypeParameterType{Parameter: typeParam}
		}

		// --- NEW: Check if this is a forward reference to the current generic class ---
		if c.currentForwardRef != nil && node.Value == c.currentForwardRef.ClassName {
			debugPrintf("// [Checker resolveTypeAnno Ident] Resolved '%s' as forward reference to current generic class\n", node.Value)
			return c.currentForwardRef
		}

		// --- NEW: Check for recursive type alias resolution ---
		if c.resolvingTypeAliases[node.Value] {
			debugPrintf("// [Checker resolveTypeAnno Ident] Detected recursive reference to '%s', creating placeholder\n", node.Value)
			// Create a placeholder type for forward reference
			// This will be resolved later when the full type is available
			return &types.TypeAliasForwardReference{
				AliasName: node.Value,
			}
		}

		// --- UPDATED: Prioritize alias resolution ---
		// 1. Attempt to resolve as a type alias in the environment
		resolvedAlias, found := c.env.ResolveType(node.Value)
		if found {
			debugPrintf("// [Checker resolveTypeAnno Ident] Resolved '%s' as alias: %T\n", node.Value, resolvedAlias) // ADDED DEBUG
			return resolvedAlias                                                                                      // Successfully resolved as an alias
		}
		debugPrintf("// [Checker resolveTypeAnno Ident] '%s' not found as alias, checking primitives...\n", node.Value) // ADDED DEBUG

		// 2. If not found as an alias, check against known primitive type names
		switch node.Value {
		case "number":
			debugPrintf("// [Checker resolveTypeAnno Ident] Matched primitive: number\n") // ADDED DEBUG
			return types.Number                                                           // Returns the package-level variable 'types.Number'
		case "string":
			debugPrintf("// [Checker resolveTypeAnno Ident] Matched primitive: string\n") // ADDED DEBUG
			return types.String
		case "boolean":
			return types.Boolean
		case "null":
			return types.Null
		case "undefined":
			return types.Undefined
		case "any":
			return types.Any
		case "unknown":
			return types.Unknown
		case "never":
			return types.Never
		case "void":
			return types.Void
		case "object":
			return types.NewObjectType()
		default:
			// 3. If neither alias nor primitive, it's an unknown type name
			debugPrintf("// [Checker resolveTypeAnno Ident] Primitive check failed for '%s', reporting error.\n", node.Value) // ADDED DEBUG
			// Use the Identifier node itself for error reporting
			c.addError(node, fmt.Sprintf("unknown type name: %s", node.Value))
			return nil // Indicate error
		}

	case *parser.UnionTypeExpression: // Added
		leftType := c.resolveTypeAnnotation(node.Left)
		rightType := c.resolveTypeAnnotation(node.Right)

		if leftType == nil || rightType == nil {
			// Error occurred resolving one of the sides
			return nil
		}

		// --- UPDATED: Use NewUnionType constructor ---
		// This handles flattening and duplicate removal automatically.
		return types.NewUnionType(leftType, rightType)

	case *parser.IntersectionTypeExpression: // NEW: Handle intersection types
		leftType := c.resolveTypeAnnotation(node.Left)
		rightType := c.resolveTypeAnnotation(node.Right)

		if leftType == nil || rightType == nil {
			// Error occurred resolving one of the sides
			return nil
		}

		// Use NewIntersectionType constructor
		// This handles flattening and simplification automatically.
		return types.NewIntersectionType(leftType, rightType)

	// --- NEW: Handle ArrayTypeExpression ---
	case *parser.ArrayTypeExpression:
		elemType := c.resolveTypeAnnotation(node.ElementType)
		if elemType == nil {
			return nil // Error resolving element type
		}
		arrayType := &types.ArrayType{ElementType: elemType}
		// No need to set computed type on the type annotation node itself
		return arrayType

	// --- NEW: Handle TupleTypeExpression ---
	case *parser.TupleTypeExpression:
		elementTypes := []types.Type{}
		for _, elemNode := range node.ElementTypes {
			elemType := c.resolveTypeAnnotation(elemNode)
			if elemType == nil {
				return nil // Error resolving element type
			}
			elementTypes = append(elementTypes, elemType)
		}

		// Handle rest element if present
		var restElementType types.Type
		if node.RestElement != nil {
			restType := c.resolveTypeAnnotation(node.RestElement)
			if restType == nil {
				return nil // Error resolving rest element type
			}

			// Validate that rest element type is an array type
			if _, isArrayType := restType.(*types.ArrayType); !isArrayType {
				c.addError(node.RestElement, fmt.Sprintf("rest element in tuple type must be an array type, got '%s'", restType.String()))
				// Use any[] as fallback
				restElementType = &types.ArrayType{ElementType: types.Any}
			} else {
				restElementType = restType
			}
		}

		tupleType := &types.TupleType{
			ElementTypes:     elementTypes,
			OptionalElements: node.OptionalFlags, // Copy the optional flags directly
			RestElementType:  restElementType,
		}
		return tupleType

	// --- NEW: Handle Literal Type Nodes ---
	case *parser.StringLiteral:
		return &types.LiteralType{Value: vm.String(node.Value)}
	case *parser.NumberLiteral:
		return &types.LiteralType{Value: vm.Number(node.Value)}
	case *parser.BooleanLiteral:
		return &types.LiteralType{Value: vm.BooleanValue(node.Value)}
	case *parser.NullLiteral:
		return types.Null
	case *parser.UndefinedLiteral:
		return types.Undefined
	// --- End Literal Type Nodes ---

	// --- NEW: Handle FunctionTypeExpression --- <<<
	case *parser.FunctionTypeExpression:
		return c.resolveFunctionTypeSignature(node)

	// --- NEW: Handle ObjectTypeExpression ---
	case *parser.ObjectTypeExpression:
		return c.resolveObjectTypeSignature(node)

	// --- NEW: Handle GenericTypeRef ---
	case *parser.GenericTypeRef:
		// For Phase 1, we only support built-in generic types
		switch node.Name.Value {
		case "Array":
			if len(node.TypeArguments) != 1 {
				c.addError(node, "Array requires exactly one type argument")
				return nil
			}
			elemType := c.resolveTypeAnnotation(node.TypeArguments[0])
			if elemType == nil {
				return nil // Error already reported
			}
			return &types.ArrayType{ElementType: elemType}

		case "Promise":
			if len(node.TypeArguments) != 1 {
				c.addError(node, "Promise requires exactly one type argument")
				return nil
			}
			valueType := c.resolveTypeAnnotation(node.TypeArguments[0])
			if valueType == nil {
				return nil // Error already reported
			}
			// For now, return a simple ObjectType with Promise-like structure
			// In a full implementation, we'd have a dedicated PromiseType
			promiseType := types.NewObjectType()
			promiseType.WithProperty("then", types.Any) // Simplified for now
			promiseType.WithProperty("catch", types.Any)
			return promiseType

		default:
			// Check if this is a forward reference to the current generic class
			var baseType types.Type
			var exists bool

			if c.currentForwardRef != nil && node.Name.Value == c.currentForwardRef.ClassName {
				debugPrintf("// [Checker resolveTypeAnno GenericTypeRef] Resolved '%s' as forward reference to current generic class\n", node.Name.Value)
				baseType = c.currentForwardRef
				exists = true
			} else {
				// Check if this is a recursive reference to the type alias being defined
				if c.resolvingTypeAliases[node.Name.Value] {
					debugPrintf("// [Checker resolveTypeAnno GenericTypeRef] Detected recursive generic reference to '%s', creating placeholder\n", node.Name.Value)
					// Create a placeholder for the recursive reference
					// We'll use a special GenericForwardReference type that includes type arguments
					return &types.GenericTypeAliasForwardReference{
						AliasName:     node.Name.Value,
						TypeArguments: make([]types.Type, len(node.TypeArguments)), // Placeholder args
					}
				}
				
				debugPrintf("// [Checker resolveTypeAnno GenericTypeRef] Resolving generic type '%s' in environment. Current resolving: %v\n", node.Name.Value, c.resolvingTypeAliases)
				
				// Check if this is a user-defined generic type
				baseType, exists = c.env.ResolveType(node.Name.Value)
			}

			if !exists {
				// During type alias resolution, if we encounter a forward reference to another
				// type alias that might be defined later, create a placeholder for it
				// This allows mutual recursion between type aliases
				debugPrintf("// [Checker resolveTypeAnno GenericTypeRef] Creating forward reference for unknown type '%s'\n", node.Name.Value)
				return &types.GenericTypeAliasForwardReference{
					AliasName:     node.Name.Value,
					TypeArguments: make([]types.Type, len(node.TypeArguments)), // Placeholder args
				}
			}

			// Check if it's a ForwardReferenceType (self-reference during class definition)
			if forwardRefType, ok := baseType.(*types.ForwardReferenceType); ok {
				// For forward references, we can't do full instantiation yet
				// Return a placeholder that includes the type arguments for later resolution
				debugPrintf("// [Checker resolveTypeAnno GenericTypeRef] Creating forward reference placeholder for '%s' with %d type args\n",
					node.Name.Value, len(node.TypeArguments))

				// For now, just return the forward reference itself
				// In a more complete implementation, we'd create a ForwardReferenceInstance
				return forwardRefType
			}

			// Check if it's a GenericType
			if genericType, ok := baseType.(*types.GenericType); ok {
				// Validate type argument count
				if len(node.TypeArguments) != len(genericType.TypeParameters) {
					c.addError(node, fmt.Sprintf("Generic type '%s' expects %d type arguments, got %d",
						node.Name.Value, len(genericType.TypeParameters), len(node.TypeArguments)))
					return nil
				}

				// Resolve type arguments
				typeArgs := make([]types.Type, len(node.TypeArguments))
				for i, argExpr := range node.TypeArguments {
					argType := c.resolveTypeAnnotation(argExpr)
					if argType == nil {
						return nil // Error already reported
					}
					typeArgs[i] = argType
				}

				// Instantiate the generic type
				return c.instantiateGenericType(genericType, typeArgs, node.TypeArguments)
			} else {
				c.addError(node, fmt.Sprintf("Type '%s' is not a generic type", node.Name.Value))
				return nil
			}
		}

	// --- NEW: Handle ConstructorTypeExpression ---
	case *parser.ConstructorTypeExpression:
		return c.resolveConstructorTypeSignature(node)

	case *parser.KeyofTypeExpression:
		return c.resolveKeyofTypeExpression(node)

	case *parser.TypePredicateExpression:
		return c.resolveTypePredicateExpression(node)

	case *parser.MappedTypeExpression:
		return c.resolveMappedTypeExpression(node)

	case *parser.IndexedAccessTypeExpression:
		return c.resolveIndexedAccessTypeExpression(node)

	case *parser.ConditionalTypeExpression:
		return c.resolveConditionalTypeExpression(node)

	case *parser.TemplateLiteralTypeExpression:
		return c.resolveTemplateLiteralTypeExpression(node)

	default:
		// If we get here, the parser created a node type that resolveTypeAnnotation doesn't handle yet.
		c.addError(node, fmt.Sprintf("unsupported type annotation node: %T", node))
		return nil // Indicate error
	}
}

// --- NEW: Helper to resolve FunctionTypeExpression nodes ---
func (c *Checker) resolveFunctionTypeSignature(node *parser.FunctionTypeExpression) types.Type {
	paramTypes := []types.Type{}
	for _, paramNode := range node.Parameters {
		paramType := c.resolveTypeAnnotation(paramNode)
		if paramType == nil {
			// Error should have been added by resolveTypeAnnotation
			return nil // Indicate error by returning nil
		}
		paramTypes = append(paramTypes, paramType)
	}

	// Handle rest parameter if present
	var restParameterType types.Type
	if node.RestParameter != nil {
		resolvedRestType := c.resolveTypeAnnotation(node.RestParameter)
		if resolvedRestType != nil {
			// Validate that rest parameter type is an array type
			if _, isArrayType := resolvedRestType.(*types.ArrayType); !isArrayType {
				c.addError(node.RestParameter, fmt.Sprintf("rest parameter type must be an array type, got '%s'", resolvedRestType.String()))
				resolvedRestType = &types.ArrayType{ElementType: types.Any}
			}
		} else {
			// Default to any[] if resolution failed
			resolvedRestType = &types.ArrayType{ElementType: types.Any}
		}
		restParameterType = resolvedRestType
	}

	returnType := c.resolveTypeAnnotation(node.ReturnType)
	if returnType == nil {
		// Error should have been added by resolveTypeAnnotation
		return nil // Indicate error by returning nil
	}

	// Create signature
	sig := &types.Signature{
		ParameterTypes:    paramTypes,
		ReturnType:        returnType,
		IsVariadic:        node.RestParameter != nil,
		RestParameterType: restParameterType,
		// Note: Function type expressions don't track optional parameters
		// They are just type signatures, not parameter declarations
	}

	// Create a unified ObjectType with call signature
	return types.NewFunctionType(sig)
}

// --- NEW: Helper to resolve ObjectTypeExpression nodes ---
func (c *Checker) resolveObjectTypeSignature(node *parser.ObjectTypeExpression) types.Type {
	properties := make(map[string]types.Type)
	optionalProperties := make(map[string]bool)
	var callSignatures []*types.Signature
	var indexSignatures []*types.IndexSignature

	for _, prop := range node.Properties {
		if prop.IsCallSignature {
			// Handle call signature like (param: type): returnType
			var paramTypes []types.Type
			for _, paramNode := range prop.Parameters {
				paramType := c.resolveTypeAnnotation(paramNode)
				if paramType == nil {
					paramType = types.Any
				}
				paramTypes = append(paramTypes, paramType)
			}

			returnType := c.resolveTypeAnnotation(prop.ReturnType)
			if returnType == nil {
				returnType = types.Any
			}

			// Create a signature
			sig := &types.Signature{
				ParameterTypes: paramTypes,
				ReturnType:     returnType,
				// Note: Object type call signatures don't track optional parameters for now
			}

			callSignatures = append(callSignatures, sig)
		} else if prop.IsIndexSignature {
			// Handle index signature like [key: string]: Type
			// For now, we'll add them to IndexSignatures field of ObjectType
			keyType := c.resolveTypeAnnotation(prop.KeyType)
			if keyType == nil {
				keyType = types.Any
			}
			valueType := c.resolveTypeAnnotation(prop.ValueType)
			if valueType == nil {
				valueType = types.Any
			}

			indexSig := &types.IndexSignature{
				KeyType:   keyType,
				ValueType: valueType,
			}

			// We'll add this to the ObjectType's IndexSignatures field
			debugPrintf("// [Checker ObjectType] Index signature [%s: %s]: %s\n",
				prop.KeyName.Value, keyType.String(), valueType.String())

			// Add to our collected index signatures
			indexSignatures = append(indexSignatures, indexSig)

		} else if prop.Name != nil {
			// Regular property or method
			propType := c.resolveTypeAnnotation(prop.Type)
			if propType == nil {
				propType = types.Any
			}
			properties[prop.Name.Value] = propType
			if prop.Optional {
				optionalProperties[prop.Name.Value] = true
			}
			debugPrintf("// [Checker ObjectType] Property '%s'%s: %s\n",
				prop.Name.Value,
				func() string {
					if prop.Optional {
						return "?"
					} else {
						return ""
					}
				}(),
				propType.String())
		}
		// Skip properties with nil names that aren't call signatures
	}

	// Create a unified ObjectType
	objectType := &types.ObjectType{
		Properties:         properties,
		OptionalProperties: optionalProperties,
		CallSignatures:     callSignatures,
		IndexSignatures:    indexSignatures,
	}

	// If it's a pure callable object with no properties and exactly one signature,
	// we can make it a pure function type for better type display
	if len(properties) == 0 && len(callSignatures) == 1 {
		// Create a fresh object to ensure we get a "pure function"
		// (the IsPureFunction helper method will return true)
		return types.NewFunctionType(callSignatures[0])
	}

	return objectType
}

// --- NEW: Helper to resolve ConstructorTypeExpression nodes ---
func (c *Checker) resolveConstructorTypeSignature(node *parser.ConstructorTypeExpression) types.Type {
	paramTypes := []types.Type{}
	for _, paramNode := range node.Parameters {
		paramType := c.resolveTypeAnnotation(paramNode)
		if paramType == nil {
			// Error should have been added by resolveTypeAnnotation
			return nil // Indicate error by returning nil
		}
		paramTypes = append(paramTypes, paramType)
	}

	constructedType := c.resolveTypeAnnotation(node.ReturnType)
	if constructedType == nil {
		// Error should have been added by resolveTypeAnnotation
		return nil // Indicate error by returning nil
	}

	// Create signature
	sig := &types.Signature{
		ParameterTypes: paramTypes,
		ReturnType:     constructedType,
		// Note: Constructor type expressions don't track optional parameters
	}

	// Create a unified ObjectType with constructor signature
	return types.NewConstructorType(sig)
}

// Resolves parameter and return type annotations within the given environment.
// Returns a types.Signature for the unified type system
func (c *Checker) resolveFunctionLiteralSignature(node *parser.FunctionLiteral, env *Environment) *types.Signature {
	paramTypes := []types.Type{}
	var optionalParams []bool

	// Create a temporary environment that will progressively accumulate parameters
	// This allows later parameters to reference earlier ones in their default values
	tempEnv := NewEnclosedEnvironment(env) // Create child environment

	for _, paramNode := range node.Parameters {
		var resolvedParamType types.Type
		if paramNode.TypeAnnotation != nil {
			// Temporarily use the provided environment for resolving the annotation
			originalEnv := c.env
			c.env = env
			resolvedParamType = c.resolveTypeAnnotation(paramNode.TypeAnnotation)
			c.env = originalEnv // Restore original environment
		}

		// NEW: If no type annotation but has default value, infer type from default value
		if resolvedParamType == nil && paramNode.DefaultValue != nil {
			// Use the temporary environment that includes previously defined parameters
			originalEnv := c.env
			c.env = tempEnv                 // Use progressive environment that includes earlier parameters
			c.visit(paramNode.DefaultValue) // This will set the computed type
			c.env = originalEnv             // Restore original environment

			defaultValueType := paramNode.DefaultValue.GetComputedType()
			if defaultValueType != nil {
				// Widen literal types for parameter inference (like let/const inference)
				resolvedParamType = types.GetWidenedType(defaultValueType)
				debugPrintf("// [Checker resolveFuncLitType] Inferred parameter '%s' type from default value: %s -> %s\n",
					paramNode.Name.Value, defaultValueType.String(), resolvedParamType.String())
			}
		}

		if resolvedParamType == nil {
			resolvedParamType = types.Any // Default to Any if no annotation or resolution failed
		}
		paramTypes = append(paramTypes, resolvedParamType)

		// Add this parameter to the temporary environment BEFORE checking its default value
		// This way, the next parameter's default value can reference this parameter
		// Skip 'this' parameters as they don't have names and don't go into the scope
		if !paramNode.IsThis {
			tempEnv.Define(paramNode.Name.Value, resolvedParamType, false) // false = not const
		}

		// Validate default value if present (skip if we already visited it for inference)
		if paramNode.DefaultValue != nil && paramNode.TypeAnnotation != nil {
			// Only validate if we had an explicit annotation (inference case already visited above)
			// Use the temporary environment that includes previously defined parameters
			originalEnv := c.env
			c.env = tempEnv                 // Use progressive environment that includes earlier parameters
			c.visit(paramNode.DefaultValue) // This will set the computed type
			c.env = originalEnv             // Restore original environment

			defaultValueType := paramNode.DefaultValue.GetComputedType()
			if defaultValueType != nil && !types.IsAssignable(defaultValueType, resolvedParamType) {
				c.addError(paramNode.DefaultValue, fmt.Sprintf("default value type '%s' is not assignable to parameter type '%s'", defaultValueType.String(), resolvedParamType.String()))
			}
		}

		// Parameter is optional if explicitly marked OR has a default value
		isOptional := paramNode.Optional || (paramNode.DefaultValue != nil)
		optionalParams = append(optionalParams, isOptional)
	}

	// Handle rest parameter if present
	var restParameterType types.Type
	if node.RestParameter != nil {
		var resolvedRestType types.Type
		if node.RestParameter.TypeAnnotation != nil {
			originalEnv := c.env
			c.env = env
			resolvedRestType = c.resolveTypeAnnotation(node.RestParameter.TypeAnnotation)
			c.env = originalEnv

			// Rest parameter type should be an array type
			if resolvedRestType != nil {
				if _, isArrayType := resolvedRestType.(*types.ArrayType); !isArrayType {
					c.addError(node.RestParameter.TypeAnnotation, fmt.Sprintf("rest parameter type must be an array type, got '%s'", resolvedRestType.String()))
					resolvedRestType = &types.ArrayType{ElementType: types.Any}
				}
			}
		}

		if resolvedRestType == nil {
			// Default to any[] if no annotation
			resolvedRestType = &types.ArrayType{ElementType: types.Any}
		}

		restParameterType = resolvedRestType
		node.RestParameter.ComputedType = restParameterType
	}

	var resolvedReturnType types.Type         // Keep as interface type
	var resolvedTypeFromAnnotation types.Type // Temp variable, also interface type

	funcNameForLog := "<anonymous_resolve>"
	if node.Name != nil {
		funcNameForLog = node.Name.Value
	}

	if node.ReturnTypeAnnotation != nil {
		originalEnv := c.env
		c.env = env
		debugPrintf("// [Checker resolveFuncLitType] Resolving return annotation (%T) for func '%s'\n", node.ReturnTypeAnnotation, funcNameForLog)
		// Assign to the temporary variable first
		resolvedTypeFromAnnotation = c.resolveTypeAnnotation(node.ReturnTypeAnnotation)
		debugPrintf("// [Checker resolveFuncLitType] resolveTypeAnnotation returned: Ptr=%p, Type=%T, Value=%#v for func '%s'\n", resolvedTypeFromAnnotation, resolvedTypeFromAnnotation, resolvedTypeFromAnnotation, funcNameForLog)
		// DO NOT assign to resolvedReturnType here yet
		c.env = originalEnv
	} else {
		debugPrintf("// [Checker resolveFuncLitType] No return type annotation for func '%s'\n", funcNameForLog)
		resolvedTypeFromAnnotation = nil // Ensure temp var is nil if no annotation
	}

	// Now, assign the result from the temporary variable to the main one
	resolvedReturnType = resolvedTypeFromAnnotation
	debugPrintf("// [Checker resolveFuncLitType] Assigned final resolvedReturnType: Ptr=%p, Type=%T, Value=%#v for func '%s'\n", resolvedReturnType, resolvedReturnType, resolvedReturnType, funcNameForLog) // ADDED LOG

	// The final check should now behave correctly
	if resolvedReturnType == nil {
		debugPrintf("// [Checker resolveFuncLitType] Final check: resolvedReturnType is nil for func '%s'\n", funcNameForLog)
		// resolvedReturnType = nil // It's already nil, no need to re-assign
	} else {
		debugPrintf("// [Checker resolveFuncLitType] Final check: resolvedReturnType is NOT nil (Ptr=%p) for func '%s'\n", resolvedReturnType, funcNameForLog)
	}

	// Return a unified Signature
	return &types.Signature{
		ParameterTypes:    paramTypes,
		ReturnType:        resolvedReturnType, // Use the value assigned outside the if
		OptionalParams:    optionalParams,
		IsVariadic:        node.RestParameter != nil,
		RestParameterType: restParameterType,
	}
}

// instantiateGenericType creates a concrete type by substituting type arguments
// into a GenericType's body type
func (c *Checker) instantiateGenericType(genericType *types.GenericType, typeArgs []types.Type, typeArgNodes []parser.Expression) types.Type {
	// Create debug string for type arguments
	var typeStrs []string
	for _, t := range typeArgs {
		typeStrs = append(typeStrs, t.String())
	}
	debugPrintf("// [Checker] Instantiating generic type '%s' with args [%s]\n",
		genericType.Name, strings.Join(typeStrs, ", "))

	// Validate constraints before instantiation
	for i, typeParam := range genericType.TypeParameters {
		if typeParam.Constraint != nil {
			argType := typeArgs[i]
			constraintType := typeParam.Constraint

			debugPrintf("// [Checker] Checking constraint: %s extends %s\n",
				argType.String(), constraintType.String())

			// Check if the type argument satisfies the constraint
			if !types.IsAssignable(argType, constraintType) {
				// Create a more detailed error message with proper node position
				errorMsg := fmt.Sprintf("Type '%s' does not satisfy constraint '%s' for type parameter '%s'",
					argType.String(), constraintType.String(), typeParam.Name)

				// Use the specific type argument node for accurate error positioning
				if i < len(typeArgNodes) && typeArgNodes[i] != nil {
					c.addConstraintError(typeArgNodes[i], errorMsg)
				} else {
					// Fallback to generic error if node is not available
					c.addGenericError(errorMsg)
				}
				return types.Any // Return any type to allow compilation to continue
			}
		}
	}

	// Create substitution map from type parameters to concrete types
	substitution := make(map[string]types.Type)
	for i, typeParam := range genericType.TypeParameters {
		substitution[typeParam.Name] = typeArgs[i]
	}

	// Perform type substitution on the body type
	instantiatedType := c.substituteTypes(genericType.Body, substitution)

	debugPrintf("// [Checker] Instantiated type: %s\n", instantiatedType.String())
	return instantiatedType
}

// substituteTypes recursively substitutes TypeParameterType with concrete types
func (c *Checker) substituteTypes(t types.Type, substitution map[string]types.Type) types.Type {
	if t == nil {
		return nil
	}

	switch typ := t.(type) {
	case *types.TypeParameterType:
		// Replace type parameter with concrete type
		if replacement, exists := substitution[typ.Parameter.Name]; exists {
			debugPrintf("// [Checker] Substituting %s -> %s\n", typ.Parameter.Name, replacement.String())
			return replacement
		}
		// Type parameter not found in substitution - this shouldn't happen if validation is correct
		debugPrintf("// [Checker] WARNING: Type parameter '%s' not found in substitution map\n", typ.Parameter.Name)
		return typ

	case *types.ForwardReferenceType:
		// Forward references should not appear in instantiated types, but handle gracefully
		debugPrintf("// [Checker] WARNING: ForwardReferenceType '%s' found during type substitution\n", typ.ClassName)
		// Return as-is for now - this indicates the forward reference wasn't properly resolved
		return typ

	case *types.ArrayType:
		// Recursively substitute element type
		newElementType := c.substituteTypes(typ.ElementType, substitution)
		return &types.ArrayType{ElementType: newElementType}

	case *types.ObjectType:
		// Handle ObjectType with properties and call signatures
		result := &types.ObjectType{
			Properties:         make(map[string]types.Type),
			OptionalProperties: make(map[string]bool),
		}

		// Copy and substitute property types
		for propName, propType := range typ.Properties {
			result.Properties[propName] = c.substituteTypes(propType, substitution)
		}
		for propName, isOptional := range typ.OptionalProperties {
			result.OptionalProperties[propName] = isOptional
		}

		// Copy and substitute call signatures
		if typ.CallSignatures != nil {
			result.CallSignatures = make([]*types.Signature, len(typ.CallSignatures))
			for i, sig := range typ.CallSignatures {
				newParamTypes := make([]types.Type, len(sig.ParameterTypes))
				for j, paramType := range sig.ParameterTypes {
					newParamTypes[j] = c.substituteTypes(paramType, substitution)
				}

				newReturnType := c.substituteTypes(sig.ReturnType, substitution)
				newRestParamType := c.substituteTypes(sig.RestParameterType, substitution)

				result.CallSignatures[i] = &types.Signature{
					ParameterTypes:    newParamTypes,
					ReturnType:        newReturnType,
					OptionalParams:    sig.OptionalParams, // Copy optional flags
					IsVariadic:        sig.IsVariadic,
					RestParameterType: newRestParamType,
				}
			}
		}

		// Copy construct signatures (if any)
		if typ.ConstructSignatures != nil {
			result.ConstructSignatures = make([]*types.Signature, len(typ.ConstructSignatures))
			for i, sig := range typ.ConstructSignatures {
				newParamTypes := make([]types.Type, len(sig.ParameterTypes))
				for j, paramType := range sig.ParameterTypes {
					newParamTypes[j] = c.substituteTypes(paramType, substitution)
				}

				newReturnType := c.substituteTypes(sig.ReturnType, substitution)
				newRestParamType := c.substituteTypes(sig.RestParameterType, substitution)

				result.ConstructSignatures[i] = &types.Signature{
					ParameterTypes:    newParamTypes,
					ReturnType:        newReturnType,
					OptionalParams:    sig.OptionalParams,
					IsVariadic:        sig.IsVariadic,
					RestParameterType: newRestParamType,
				}
			}
		}

		// Copy ClassMeta to preserve class instance information
		if typ.ClassMeta != nil {
			result.ClassMeta = &types.ClassMetadata{
				ClassName:         typ.ClassMeta.ClassName,
				IsClassInstance:   typ.ClassMeta.IsClassInstance,
				IsClassConstructor: typ.ClassMeta.IsClassConstructor,
				MemberAccess:      typ.ClassMeta.MemberAccess,
			}
		}

		return result

	case *types.UnionType:
		// Recursively substitute union member types
		newTypes := make([]types.Type, len(typ.Types))
		for i, memberType := range typ.Types {
			newTypes[i] = c.substituteTypes(memberType, substitution)
		}
		return types.NewUnionType(newTypes...)

	case *types.IntersectionType:
		// Recursively substitute intersection member types
		newTypes := make([]types.Type, len(typ.Types))
		for i, memberType := range typ.Types {
			newTypes[i] = c.substituteTypes(memberType, substitution)
		}
		return types.NewIntersectionType(newTypes...)

	case *types.TupleType:
		// Recursively substitute tuple element types
		newElementTypes := make([]types.Type, len(typ.ElementTypes))
		for i, elementType := range typ.ElementTypes {
			newElementTypes[i] = c.substituteTypes(elementType, substitution)
		}
		return &types.TupleType{ElementTypes: newElementTypes}

	case *types.MappedType:
		// Recursively substitute types in mapped type
		newConstraintType := c.substituteTypes(typ.ConstraintType, substitution)
		newValueType := c.substituteTypes(typ.ValueType, substitution)
		
		return &types.MappedType{
			TypeParameter:    typ.TypeParameter,    // Keep parameter name as-is
			ConstraintType:   newConstraintType,
			ValueType:        newValueType,
			OptionalModifier: typ.OptionalModifier,
			ReadonlyModifier: typ.ReadonlyModifier,
		}

	case *types.KeyofType:
		// Substitute the operand type
		newOperandType := c.substituteTypes(typ.OperandType, substitution)
		// Compute the keyof type after substitution
		return c.computeKeyofType(newOperandType)

	case *types.IndexedAccessType:
		// Substitute both object and index types
		newObjectType := c.substituteTypes(typ.ObjectType, substitution)
		newIndexType := c.substituteTypes(typ.IndexType, substitution)
		return &types.IndexedAccessType{
			ObjectType: newObjectType,
			IndexType:  newIndexType,
		}

	case *types.ConditionalType:
		// Substitute all types in conditional type
		newCheckType := c.substituteTypes(typ.CheckType, substitution)
		newExtendsType := c.substituteTypes(typ.ExtendsType, substitution)
		newTrueType := c.substituteTypes(typ.TrueType, substitution)
		newFalseType := c.substituteTypes(typ.FalseType, substitution)
		
		// Try to compute the result with substituted types
		resolvedType := c.computeConditionalType(newCheckType, newExtendsType, newTrueType, newFalseType)
		if resolvedType != nil {
			return resolvedType
		}
		
		// Return the conditional type with substituted parts
		return &types.ConditionalType{
			CheckType:   newCheckType,
			ExtendsType: newExtendsType,
			TrueType:    newTrueType,
			FalseType:   newFalseType,
		}

	case *types.TemplateLiteralType:
		// Substitute types in template literal type parts
		newParts := make([]types.TemplateLiteralPart, len(typ.Parts))
		for i, part := range typ.Parts {
			if part.IsLiteral {
				// String literals don't need substitution
				newParts[i] = part
			} else {
				// Substitute the type in type interpolation parts
				newType := c.substituteTypes(part.Type, substitution)
				newParts[i] = types.TemplateLiteralPart{
					IsLiteral: false,
					Literal:   "",
					Type:      newType,
				}
			}
		}
		
		// Try to compute the template literal type with substituted parts
		substitutedTlt := &types.TemplateLiteralType{Parts: newParts}
		computedType := c.computeTemplateLiteralType(substitutedTlt)
		if computedType != nil {
			return computedType
		}
		
		return substitutedTlt

	default:
		// For primitive types and other types that don't contain type parameters,
		// return as-is
		return typ
	}
}

// resolveKeyofTypeExpression resolves a keyof type expression to a union of string literals
func (c *Checker) resolveKeyofTypeExpression(node *parser.KeyofTypeExpression) types.Type {
	if node.Type == nil {
		c.addError(node, "keyof expression missing operand type")
		return nil
	}

	operandType := c.resolveTypeAnnotation(node.Type)
	if operandType == nil {
		// Error already reported by resolveTypeAnnotation
		return nil
	}

	// Compute the actual keyof type by extracting keys from the operand type
	return c.computeKeyofType(operandType)
}

// computeKeyofType computes the keyof type for a given type
func (c *Checker) computeKeyofType(operandType types.Type) types.Type {
	switch typ := operandType.(type) {
	case *types.ObjectType:
		// Extract property names and create string literal types
		var keyTypes []types.Type
		
		// Add regular properties
		for propName := range typ.Properties {
			keyTypes = append(keyTypes, &types.LiteralType{
				Value: vm.String(propName),
			})
		}
		
		// If there are no properties, keyof should be never
		if len(keyTypes) == 0 {
			return types.Never
		}
		
		// If there's only one key, return the literal type directly
		if len(keyTypes) == 1 {
			return keyTypes[0]
		}
		
		// Return union of all key literal types
		return types.NewUnionType(keyTypes...)
		
	default:
		// Handle special cases
		if operandType == types.Any {
			// keyof any should be string | number | symbol (simplified to string for now)
			return types.String
		}
		// For non-object types, keyof typically resolves to never
		// TODO: Handle other types like arrays (which should include numeric indices)
		return types.Never
	}
}

// resolveTypePredicateExpression resolves a type predicate expression to a TypePredicateType
func (c *Checker) resolveTypePredicateExpression(node *parser.TypePredicateExpression) types.Type {
	if node.Parameter == nil {
		c.addError(node, "type predicate missing parameter name")
		return nil
	}

	if node.Type == nil {
		c.addError(node, "type predicate missing type")
		return nil
	}

	predicateType := c.resolveTypeAnnotation(node.Type)
	if predicateType == nil {
		// Error already reported by resolveTypeAnnotation
		return nil
	}

	return &types.TypePredicateType{
		ParameterName: node.Parameter.Value,
		Type:          predicateType,
	}
}

// resolveMappedTypeExpression resolves a mapped type expression to a MappedType
func (c *Checker) resolveMappedTypeExpression(node *parser.MappedTypeExpression) types.Type {
	if node.TypeParameter == nil {
		c.addError(node, "mapped type missing type parameter")
		return nil
	}

	if node.ConstraintType == nil {
		c.addError(node, "mapped type missing constraint type")
		return nil
	}

	if node.ValueType == nil {
		c.addError(node, "mapped type missing value type")
		return nil
	}

	// Resolve constraint type (the type being iterated over)
	constraintType := c.resolveTypeAnnotation(node.ConstraintType)
	if constraintType == nil {
		// Error already reported by resolveTypeAnnotation
		return nil
	}

	// Create a temporary environment with the type parameter in scope
	// This allows expressions like T[P] to resolve P correctly
	originalEnv := c.env
	tempEnv := NewEnclosedEnvironment(c.env)
	
	// Add the type parameter to the temporary environment
	// For now, we'll represent it as a type parameter type
	typeParam := &types.TypeParameter{
		Name:       node.TypeParameter.Value,
		Constraint: constraintType, // The constraint is what P extends/iterates over
	}
	tempEnv.DefineTypeParameter(node.TypeParameter.Value, typeParam)
	
	// Switch to the temporary environment for resolving the value type
	c.env = tempEnv
	valueType := c.resolveTypeAnnotation(node.ValueType)
	c.env = originalEnv // Restore original environment
	
	if valueType == nil {
		// Error already reported by resolveTypeAnnotation
		return nil
	}

	return &types.MappedType{
		TypeParameter:    node.TypeParameter.Value,
		ConstraintType:   constraintType,
		ValueType:        valueType,
		ReadonlyModifier: node.ReadonlyModifier,
		OptionalModifier: node.OptionalModifier,
	}
}

// resolveIndexedAccessTypeExpression resolves indexed access types like T[K]
func (c *Checker) resolveIndexedAccessTypeExpression(node *parser.IndexedAccessTypeExpression) types.Type {
	if node.ObjectType == nil {
		c.addError(node, "indexed access type missing object type")
		return nil
	}

	if node.IndexType == nil {
		c.addError(node, "indexed access type missing index type")
		return nil
	}

	// Resolve the object type being indexed into
	objectType := c.resolveTypeAnnotation(node.ObjectType)
	if objectType == nil {
		// Error already reported by resolveTypeAnnotation
		return nil
	}

	// Resolve the index type used for accessing
	indexType := c.resolveTypeAnnotation(node.IndexType)
	if indexType == nil {
		// Error already reported by resolveTypeAnnotation
		return nil
	}

	// Try to compute the result if possible
	resolvedType := c.computeIndexedAccessType(objectType, indexType)
	if resolvedType != nil {
		return resolvedType
	}

	// If we can't resolve it now, return an IndexedAccessType for later resolution
	return &types.IndexedAccessType{
		ObjectType: objectType,
		IndexType:  indexType,
	}
}

// resolveConditionalTypeExpression resolves a conditional type expression to a ConditionalType
func (c *Checker) resolveConditionalTypeExpression(node *parser.ConditionalTypeExpression) types.Type {
	if node.CheckType == nil {
		c.addError(node, "conditional type missing check type")
		return nil
	}

	if node.ExtendsType == nil {
		c.addError(node, "conditional type missing extends type")
		return nil
	}

	if node.TrueType == nil {
		c.addError(node, "conditional type missing true type")
		return nil
	}

	if node.FalseType == nil {
		c.addError(node, "conditional type missing false type")
		return nil
	}

	// Resolve all component types
	checkType := c.resolveTypeAnnotation(node.CheckType)
	if checkType == nil {
		// Error already reported by resolveTypeAnnotation
		return nil
	}

	extendsType := c.resolveTypeAnnotation(node.ExtendsType)
	if extendsType == nil {
		// Error already reported by resolveTypeAnnotation
		return nil
	}

	trueType := c.resolveTypeAnnotation(node.TrueType)
	if trueType == nil {
		// Error already reported by resolveTypeAnnotation
		return nil
	}

	falseType := c.resolveTypeAnnotation(node.FalseType)
	if falseType == nil {
		// Error already reported by resolveTypeAnnotation
		return nil
	}

	// Always return a ConditionalType for later resolution/substitution
	// The computation will happen during type substitution when generics are instantiated
	return &types.ConditionalType{
		CheckType:   checkType,
		ExtendsType: extendsType,
		TrueType:    trueType,
		FalseType:   falseType,
	}
}

// computeConditionalType computes the result of a conditional type like T extends U ? X : Y
func (c *Checker) computeConditionalType(checkType, extendsType, trueType, falseType types.Type) types.Type {
	// For now, we'll implement basic conditional type resolution
	// This can be expanded to handle more complex cases later
	
	debugPrintf("// [ConditionalType] Checking if %s extends %s\n", checkType.String(), extendsType.String())
	
	// Check if checkType extends extendsType (is assignable to it)
	if types.IsAssignable(checkType, extendsType) {
		debugPrintf("// [ConditionalType] YES: %s extends %s -> %s\n", checkType.String(), extendsType.String(), trueType.String())
		return trueType
	} else {
		debugPrintf("// [ConditionalType] NO: %s does not extend %s -> %s\n", checkType.String(), extendsType.String(), falseType.String())
		return falseType
	}
}

// computeIndexedAccessType computes the result of an indexed access type like T[K]
func (c *Checker) computeIndexedAccessType(objectType, indexType types.Type) types.Type {
	// Handle object types with specific string literal keys
	if objType, ok := objectType.(*types.ObjectType); ok {
		// Case: Object["propertyName"] where "propertyName" is a string literal
		if literalType, ok := indexType.(*types.LiteralType); ok {
			if literalType.Value.Type() == vm.TypeString {
				strVal := literalType.Value.AsString()
				// Look up the property directly
				if propType, exists := objType.Properties[strVal]; exists {
					return propType
				}
				// Property doesn't exist - this could be an error or return never/undefined
				// For now, return nil to indicate it couldn't be resolved
				return nil
			}
		}

		// Case: Object[keyof Object] - return union of all property types
		if keyofType, ok := indexType.(*types.KeyofType); ok {
			if keyofType.OperandType.Equals(objectType) {
				// Collect all property types
				var propTypes []types.Type
				for _, propType := range objType.Properties {
					propTypes = append(propTypes, propType)
				}
				if len(propTypes) == 0 {
					return types.Never // No properties means never
				}
				if len(propTypes) == 1 {
					return propTypes[0] // Single property type
				}
				return types.NewUnionType(propTypes...) // Union of all property types
			}
		}

		// Case: Object[union of string literals] - return union of corresponding property types
		if unionType, ok := indexType.(*types.UnionType); ok {
			var resultTypes []types.Type
			for _, memberType := range unionType.Types {
				if literalType, ok := memberType.(*types.LiteralType); ok {
					if literalType.Value.Type() == vm.TypeString {
						strVal := literalType.Value.AsString()
						if propType, exists := objType.Properties[strVal]; exists {
							resultTypes = append(resultTypes, propType)
						}
					}
				}
			}
			if len(resultTypes) == 0 {
				return nil // Couldn't resolve any properties
			}
			if len(resultTypes) == 1 {
				return resultTypes[0]
			}
			return types.NewUnionType(resultTypes...)
		}
	}

	// TODO: Handle other cases like:
	// - Array[number] should return the element type
	// - Tuple[number] should return union of tuple element types
	// - Generic type parameters T[K] with constraints

	// For now, return nil to indicate it couldn't be resolved immediately
	return nil
}

// expandMappedType expands a mapped type to a concrete ObjectType
// Example: { [P in keyof Person]?: Person[P] } → { name?: string; age?: number }
func (c *Checker) expandMappedType(mappedType *types.MappedType) types.Type {
	if mappedType == nil {
		return nil
	}

	// Get the constraint type (what we're iterating over)
	constraintType := mappedType.ConstraintType
	if constraintType == nil {
		return nil
	}

	// Handle keyof constraint: [P in keyof SomeType]
	var iterationKeys []types.Type
	if keyofType, ok := constraintType.(*types.KeyofType); ok {
		// Get the keys from the keyof operand
		operandType := keyofType.OperandType
		if objType, ok := operandType.(*types.ObjectType); ok {
			// Extract all property names as literal types
			for propName := range objType.Properties {
				iterationKeys = append(iterationKeys, &types.LiteralType{
					Value: vm.String(propName),
				})
			}
		} else if operandType == types.Any {
			// For keyof any, we can't enumerate specific keys, so this mapped type
			// should act like any for property access - return Any
			return types.Any
		}
	} else if unionType, ok := constraintType.(*types.UnionType); ok {
		// Handle direct union constraint: [P in "name" | "age"]
		iterationKeys = unionType.Types
	} else if literalType, ok := constraintType.(*types.LiteralType); ok {
		// Handle single literal constraint: [P in "name"]
		iterationKeys = []types.Type{literalType}
	} else if constraintType == types.String {
		// Handle case where constraint is just 'string' (from keyof any)
		// This means we're mapping over all possible string keys, so return Any
		return types.Any
	} else {
		// Unsupported constraint type for now
		return nil
	}

	if len(iterationKeys) == 0 {
		return nil
	}

	// Create the expanded object type
	properties := make(map[string]types.Type)
	optionalProperties := make(map[string]bool)

	// For each key in the iteration, compute the resulting property
	for _, keyType := range iterationKeys {
		literalType, ok := keyType.(*types.LiteralType)
		if !ok || literalType.Value.Type() != vm.TypeString {
			continue // Skip non-string keys for now
		}

		keyName := literalType.Value.AsString()

		// Compute the value type for this property
		// We need to substitute P with the current key in the value type
		valueType := c.substituteTypeParameterInType(
			mappedType.ValueType,
			mappedType.TypeParameter,
			keyType,
		)

		if valueType != nil {
			properties[keyName] = valueType

			// Handle optional modifier
			if mappedType.OptionalModifier == "+" || mappedType.OptionalModifier == "" {
				// Make property optional (default behavior for ? modifier)
				optionalProperties[keyName] = true
			}
			// Note: "-" modifier would make required, but that's advanced
		}
	}

	// Create the expanded object type
	return &types.ObjectType{
		Properties:         properties,
		OptionalProperties: optionalProperties,
		CallSignatures:     []*types.Signature{}, // Mapped types don't create call signatures
		IndexSignatures:    []*types.IndexSignature{}, // TODO: Handle index signatures if needed
	}
}

// substituteTypeParameterInType substitutes a type parameter with a concrete type
// This is used when expanding mapped types to replace P with specific literal types
func (c *Checker) substituteTypeParameterInType(targetType types.Type, paramName string, replacement types.Type) types.Type {
	if targetType == nil {
		return nil
	}

	switch typ := targetType.(type) {
	case *types.TypeParameterType:
		// If this is the type parameter we're looking for, replace it
		if typ.Parameter != nil && typ.Parameter.Name == paramName {
			return replacement
		}
		return targetType

	case *types.IndexedAccessType:
		// Handle T[P] where P is the type parameter being substituted
		objectType := c.substituteTypeParameterInType(typ.ObjectType, paramName, replacement)
		indexType := c.substituteTypeParameterInType(typ.IndexType, paramName, replacement)
		
		// Try to resolve the indexed access with the substituted types
		resolvedType := c.computeIndexedAccessType(objectType, indexType)
		if resolvedType != nil {
			return resolvedType
		}
		
		// If we can't resolve it, return a new IndexedAccessType with substituted parts
		return &types.IndexedAccessType{
			ObjectType: objectType,
			IndexType:  indexType,
		}

	case *types.UnionType:
		// Recursively substitute in union members
		var substitutedTypes []types.Type
		for _, memberType := range typ.Types {
			substituted := c.substituteTypeParameterInType(memberType, paramName, replacement)
			if substituted != nil {
				substitutedTypes = append(substitutedTypes, substituted)
			}
		}
		if len(substitutedTypes) == 0 {
			return nil
		}
		if len(substitutedTypes) == 1 {
			return substitutedTypes[0]
		}
		return types.NewUnionType(substitutedTypes...)

	default:
		// For other types (primitives, objects, etc.), no substitution needed
		return targetType
	}
}

// isAssignableWithExpansion checks if source can be assigned to target,
// but first expands any mapped types to concrete object types
func (c *Checker) isAssignableWithExpansion(source, target types.Type) bool {
	debugPrintf("// [Checker] isAssignableWithExpansion: source=%T target=%T\n", source, target)
	debugPrintf("// [Checker] source: %s\n", source.String())
	debugPrintf("// [Checker] target: %s\n", target.String())
	
	// Expand target if it's a mapped type or instantiated type containing a mapped type
	expandedTarget := c.expandIfMappedType(target)

	// Expand source if it's a mapped type (less common but possible)
	expandedSource := c.expandIfMappedType(source)

	// Use the standard assignability check with expanded types
	result := types.IsAssignable(expandedSource, expandedTarget)
	debugPrintf("// [Checker] isAssignableWithExpansion result: %v\n", result)
	return result
}

// expandIfMappedType expands a type if it's a mapped type or contains a mapped type
func (c *Checker) expandIfMappedType(typ types.Type) types.Type {
	if typ == nil {
		return typ
	}

	debugPrintf("// [Checker] expandIfMappedType called with type: %T %s\n", typ, typ.String())

	// Direct mapped type
	if mappedType, ok := typ.(*types.MappedType); ok {
		debugPrintf("// [Checker] Found direct mapped type: %s\n", mappedType.String())
		debugPrintf("// [Checker] Constraint: %T %s\n", mappedType.ConstraintType, mappedType.ConstraintType.String())
		debugPrintf("// [Checker] ValueType: %T %s\n", mappedType.ValueType, mappedType.ValueType.String())
		expanded := c.expandMappedType(mappedType)
		if expanded != nil {
			debugPrintf("// [Checker] Direct mapped type expanded to: %s\n", expanded.String())
			return expanded
		}
		debugPrintf("// [Checker] Direct mapped type expansion failed\n")
		return typ
	}

	// Instantiated type that might contain a mapped type
	if instantiated, ok := typ.(*types.InstantiatedType); ok {
		debugPrintf("// [Checker] Found InstantiatedType, checking body...\n")
		// Check if the instantiated type's body is a mapped type
		if instantiated.Generic != nil && instantiated.Generic.Body != nil {
			debugPrintf("// [Checker] InstantiatedType body: %T %s\n", instantiated.Generic.Body, instantiated.Generic.Body.String())
			if mappedType, ok := instantiated.Generic.Body.(*types.MappedType); ok {
				debugPrintf("// [Checker] InstantiatedType contains mapped type, substituting...\n")
				// We need to substitute the type arguments in the mapped type
				substitutedMappedType := c.substituteMappedType(mappedType, instantiated.Generic.TypeParameters, instantiated.TypeArguments)
				if substitutedMappedType != nil {
					debugPrintf("// [Checker] Substituted mapped type: %s\n", substitutedMappedType.String())
					expanded := c.expandMappedType(substitutedMappedType)
					if expanded != nil {
						debugPrintf("// [Checker] InstantiatedType expanded to: %s\n", expanded.String())
						return expanded
					}
				}
			}
		}
	}

	debugPrintf("// [Checker] No expansion performed, returning original type\n")
	return typ
}

// substituteMappedType substitutes type arguments into a mapped type
func (c *Checker) substituteMappedType(mappedType *types.MappedType, typeParams []*types.TypeParameter, typeArgs []types.Type) *types.MappedType {
	if mappedType == nil || len(typeParams) != len(typeArgs) {
		return mappedType
	}

	// Create substitution map
	substitutions := make(map[string]types.Type)
	for i, param := range typeParams {
		if i < len(typeArgs) {
			substitutions[param.Name] = typeArgs[i]
		}
	}

	// Substitute in constraint type
	substitutedConstraint := c.substituteInType(mappedType.ConstraintType, substitutions)
	
	// Substitute in value type
	substitutedValue := c.substituteInType(mappedType.ValueType, substitutions)

	return &types.MappedType{
		TypeParameter:    mappedType.TypeParameter,
		ConstraintType:   substitutedConstraint,
		ValueType:        substitutedValue,
		ReadonlyModifier: mappedType.ReadonlyModifier,
		OptionalModifier: mappedType.OptionalModifier,
	}
}

// substituteInType performs type substitution based on a substitution map
func (c *Checker) substituteInType(typ types.Type, substitutions map[string]types.Type) types.Type {
	if typ == nil {
		return nil
	}

	switch t := typ.(type) {
	case *types.TypeParameterType:
		if t.Parameter != nil {
			if replacement, exists := substitutions[t.Parameter.Name]; exists {
				return replacement
			}
		}
		return typ

	case *types.KeyofType:
		substitutedOperand := c.substituteInType(t.OperandType, substitutions)
		return &types.KeyofType{OperandType: substitutedOperand}

	case *types.IndexedAccessType:
		substitutedObject := c.substituteInType(t.ObjectType, substitutions)
		substitutedIndex := c.substituteInType(t.IndexType, substitutions)
		return &types.IndexedAccessType{
			ObjectType: substitutedObject,
			IndexType:  substitutedIndex,
		}

	default:
		return typ
	}
}

// resolveTemplateLiteralTypeExpression resolves a template literal type expression to a TemplateLiteralType
func (c *Checker) resolveTemplateLiteralTypeExpression(node *parser.TemplateLiteralTypeExpression) types.Type {
	if node == nil || len(node.Parts) == 0 {
		c.addError(node, "empty template literal type")
		return nil
	}

	var parts []types.TemplateLiteralPart

	for i, part := range node.Parts {
		if i%2 == 0 {
			// Even indices are string parts
			if stringPart, ok := part.(*parser.TemplateStringPart); ok {
				parts = append(parts, types.TemplateLiteralPart{
					IsLiteral: true,
					Literal:   stringPart.Value,
					Type:      nil,
				})
			} else {
				c.addError(node, "expected string part in template literal type")
				return nil
			}
		} else {
			// Odd indices are type expressions
			if typeExpr, ok := part.(parser.Expression); ok {
				resolvedType := c.resolveTypeAnnotation(typeExpr)
				if resolvedType == nil {
					// Error already reported by resolveTypeAnnotation
					return nil
				}
				parts = append(parts, types.TemplateLiteralPart{
					IsLiteral: false,
					Literal:   "",
					Type:      resolvedType,
				})
			} else {
				c.addError(node, "expected type expression in template literal type interpolation")
				return nil
			}
		}
	}

	// Try to compute the template literal type to a concrete string literal
	computedType := c.computeTemplateLiteralType(&types.TemplateLiteralType{Parts: parts})
	if computedType != nil {
		return computedType
	}

	return &types.TemplateLiteralType{
		Parts: parts,
	}
}

// computeTemplateLiteralType attempts to compute a template literal type to a concrete string literal
// Returns nil if the template contains non-literal types that can't be computed
func (c *Checker) computeTemplateLiteralType(tlt *types.TemplateLiteralType) types.Type {
	var result strings.Builder
	
	for _, part := range tlt.Parts {
		if part.IsLiteral {
			// String literal part - add directly
			result.WriteString(part.Literal)
		} else {
			// Type interpolation part - try to extract string literal
			if literalType, ok := part.Type.(*types.LiteralType); ok {
				if literalType.Value.Type() == vm.TypeString {
					// It's a string literal - add its value
					result.WriteString(literalType.Value.AsString())
				} else {
					// Non-string literal - can't compute
					return nil
				}
			} else {
				// Non-literal type - can't compute to concrete string
				return nil
			}
		}
	}
	
	// All parts were computable - return as string literal type
	computedValue := result.String()
	return &types.LiteralType{
		Value: vm.String(computedValue),
	}
}

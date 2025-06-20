package checker

import (
	"fmt"
	"strings"
	"paserati/pkg/parser"
	"paserati/pkg/types"
	"paserati/pkg/vm"
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
			// Check if this is a user-defined generic type
			baseType, exists := c.env.ResolveType(node.Name.Value)
			if !exists {
				c.addError(node, fmt.Sprintf("Unknown generic type '%s'", node.Name.Value))
				return nil
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
				return c.instantiateGenericType(genericType, typeArgs)
			} else {
				c.addError(node, fmt.Sprintf("Type '%s' is not a generic type", node.Name.Value))
				return nil
			}
		}

	// --- NEW: Handle ConstructorTypeExpression ---
	case *parser.ConstructorTypeExpression:
		return c.resolveConstructorTypeSignature(node)

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
		} else if prop.Name != nil {
			// Regular property or method
			propType := c.resolveTypeAnnotation(prop.Type)
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
func (c *Checker) instantiateGenericType(genericType *types.GenericType, typeArgs []types.Type) types.Type {
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
				// Create a more detailed error message
				errorMsg := fmt.Sprintf("Type '%s' does not satisfy constraint '%s' for type parameter '%s'", 
					argType.String(), constraintType.String(), typeParam.Name)
				
				// We don't have the original AST node here, so we'll add a generic error
				// In a more complete implementation, we'd pass the node through
				c.addGenericError(errorMsg)
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
		
	default:
		// For primitive types and other types that don't contain type parameters,
		// return as-is
		return typ
	}
}

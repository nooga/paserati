package checker

import (
	"fmt"
	"paserati/pkg/parser"
	"paserati/pkg/types"
)

// --- NEW: Type Alias Statement Check ---

func (c *Checker) checkTypeAliasStatement(node *parser.TypeAliasStatement) {
	// Called ONLY during Pass 1 for hoisted aliases.

	// 1. Check if already defined (belt-and-suspenders check)
	// Note: We use c.env which IS the globalEnv during Pass 1
	if _, exists := c.env.ResolveType(node.Name.Value); exists {
		debugPrintf("// [Checker TypeAlias P1] Alias '%s' already defined? Skipping.\n", node.Name.Value)
		return // Should not happen if parser prevents duplicates
	}

	// 2. Handle generic type aliases
	if len(node.TypeParameters) > 0 {
		c.checkGenericTypeAliasStatement(node)
		return
	}

	// 3. Mark this type alias as being resolved to prevent infinite recursion
	c.resolvingTypeAliases[node.Name.Value] = true
	defer func() {
		delete(c.resolvingTypeAliases, node.Name.Value)
	}()

	// 4. Resolve the RHS type using the CURRENT (global) environment
	// This allows aliases to reference previously defined aliases in the same pass
	aliasedType := c.resolveTypeAnnotation(node.Type) // Uses c.env (globalEnv)
	if aliasedType == nil {
		debugPrintf("// [Checker TypeAlias P1] Failed to resolve type for alias '%s'. Defining as Any.\n", node.Name.Value)
		if !c.env.DefineTypeAlias(node.Name.Value, types.Any) {
			debugPrintf("// [Checker TypeAlias P1] WARNING: DefineTypeAlias failed for '%s' (as Any).\n", node.Name.Value)
		}
		return
	}

	// 5. Define the alias in the CURRENT (global) environment
	if !c.env.DefineTypeAlias(node.Name.Value, aliasedType) {
		debugPrintf("// [Checker TypeAlias P1] WARNING: DefineTypeAlias failed for '%s'.\n", node.Name.Value)
	} else {
		debugPrintf("// [Checker TypeAlias P1] Defined alias '%s' as type '%s' in env %p\n", node.Name.Value, aliasedType.String(), c.env)
	}
	// No need to set computed type on the TypeAliasStatement node itself
}

// checkGenericTypeAliasStatement handles generic type alias declarations
func (c *Checker) checkGenericTypeAliasStatement(node *parser.TypeAliasStatement) {
	debugPrintf("// [Checker TypeAlias P1] Processing generic type alias '%s' with %d type parameters\n", 
		node.Name.Value, len(node.TypeParameters))

	// Mark this type alias as being resolved to prevent infinite recursion
	c.resolvingTypeAliases[node.Name.Value] = true
	defer func() {
		delete(c.resolvingTypeAliases, node.Name.Value)
	}()

	// 1. Validate type parameters
	typeParams := make([]*types.TypeParameter, len(node.TypeParameters))
	for i, param := range node.TypeParameters {
		// Create TypeParameter type
		typeParam := &types.TypeParameter{
			Name: param.Name.Value,
		}
		
		// Handle constraint if present
		if param.Constraint != nil {
			constraintType := c.resolveTypeAnnotation(param.Constraint)
			if constraintType != nil {
				typeParam.Constraint = constraintType
			}
		}
		
		// Handle default type if present
		if param.DefaultType != nil {
			defaultType := c.resolveTypeAnnotation(param.DefaultType)
			if defaultType != nil {
				typeParam.Default = defaultType
				
				// Validate that default type satisfies constraint if both are present
				if typeParam.Constraint != nil && !types.IsAssignable(defaultType, typeParam.Constraint) {
					c.addError(param.DefaultType, fmt.Sprintf("default type '%s' does not satisfy constraint '%s'", defaultType.String(), typeParam.Constraint.String()))
				}
			}
		}
		
		typeParams[i] = typeParam
	}

	// 2. Create the body type with TypeParameterType references
	// Create a new environment with type parameters available as TypeParameterType
	genericEnv := NewEnclosedEnvironment(c.env)
	for _, typeParam := range typeParams {
		paramType := &types.TypeParameterType{
			Parameter: typeParam,
		}
		genericEnv.DefineTypeAlias(typeParam.Name, paramType)
	}
	
	// Save current environment and switch to generic environment
	savedEnv := c.env
	c.env = genericEnv
	
	// Resolve the RHS type with TypeParameterType references
	bodyType := c.resolveTypeAnnotation(node.Type)
	if bodyType == nil {
		debugPrintf("// [Checker TypeAlias P1] Failed to resolve body type for generic alias '%s'. Using Any.\n", node.Name.Value)
		bodyType = types.Any
	}

	// Restore environment
	c.env = savedEnv

	// 3. Create the GenericType
	genericType := &types.GenericType{
		Name:           node.Name.Value,
		TypeParameters: typeParams,
		Body:           bodyType,
	}

	// 4. Define in environment
	if !c.env.DefineTypeAlias(node.Name.Value, genericType) {
		debugPrintf("// [Checker TypeAlias P1] WARNING: DefineTypeAlias failed for generic type alias '%s'.\n", node.Name.Value)
	} else {
		debugPrintf("// [Checker TypeAlias P1] Defined generic type alias '%s' with %d type parameters in env %p\n",
			node.Name.Value, len(typeParams), c.env)
	}
}

// --- NEW: Interface Declaration Check ---

func (c *Checker) checkInterfaceDeclaration(node *parser.InterfaceDeclaration) {
	// Called during Pass 1 for interface declarations

	// 1. Check if already defined
	if _, exists := c.env.ResolveType(node.Name.Value); exists {
		debugPrintf("// [Checker Interface P1] Interface '%s' already defined? Skipping.\n", node.Name.Value)
		return
	}

	// 2. Handle generic interfaces
	if len(node.TypeParameters) > 0 {
		c.checkGenericInterfaceDeclaration(node)
		return
	}

	// 2. Build the ObjectType from interface properties, including inheritance
	properties := make(map[string]types.Type)
	optionalProperties := make(map[string]bool)
	var indexSignatures []*types.IndexSignature

	// First, inherit properties from extended interfaces
	for _, extendedInterfaceExpr := range node.Extends {
		// Resolve the extended interface type expression (supports both simple names and generic applications)
		extendedType := c.resolveTypeAnnotation(extendedInterfaceExpr)
		if extendedType == nil {
			c.addError(extendedInterfaceExpr, "failed to resolve extended interface type")
			continue
		}

		// The extended type should be an ObjectType (interface is stored as ObjectType)
		if extendedObjectType, ok := extendedType.(*types.ObjectType); ok {
			// Copy all properties from the extended interface
			for propName, propType := range extendedObjectType.Properties {
				properties[propName] = propType
				// Copy optional property flags
				if extendedObjectType.OptionalProperties != nil && extendedObjectType.OptionalProperties[propName] {
					optionalProperties[propName] = true
				}
			}
			debugPrintf("// [Checker Interface P1] Interface '%s' inherited %d properties from extended interface\n",
				node.Name.Value, len(extendedObjectType.Properties))
		} else if _, ok := extendedType.(*types.GenericTypeAliasForwardReference); ok {
			// Allow inheritance from generic forward references (unresolved type-only imports)
			// We can't copy properties since we don't know the structure, but we allow the syntax
			debugPrintf("// [Checker Interface P1] Interface '%s' extends unresolved generic type '%s', allowing for type-only imports\n",
				node.Name.Value, extendedType.String())
		} else {
			c.addError(extendedInterfaceExpr, fmt.Sprintf("'%s' is not an interface, cannot extend", extendedType.String()))
		}
	}

	// Then, add/override properties from this interface's declaration
	for _, prop := range node.Properties {
		if prop.IsIndexSignature {
			// Handle index signature: [key: KeyType]: ValueType
			keyType := c.resolveTypeAnnotation(prop.KeyType)
			if keyType == nil {
				debugPrintf("// [Checker Interface P1] Failed to resolve key type for index signature in interface '%s'. Using string.\n", node.Name.Value)
				keyType = types.String
			}
			
			valueType := c.resolveTypeAnnotation(prop.ValueType)
			if valueType == nil {
				debugPrintf("// [Checker Interface P1] Failed to resolve value type for index signature in interface '%s'. Using Any.\n", node.Name.Value)
				valueType = types.Any
			}
			
			indexSignature := &types.IndexSignature{
				KeyType:   keyType,
				ValueType: valueType,
			}
			indexSignatures = append(indexSignatures, indexSignature)
			
			debugPrintf("// [Checker Interface P1] Interface '%s' has index signature [%s]: %s\n", 
				node.Name.Value, keyType.String(), valueType.String())
		} else if prop.IsConstructorSignature {
			// For constructor signatures, add them as a special "new" property
			// This allows the interface to describe both instance properties and constructor behavior
			constructorType := c.resolveTypeAnnotation(prop.Type)
			if constructorType == nil {
				debugPrintf("// [Checker Interface P1] Failed to resolve constructor type in interface '%s'. Using Any.\n", node.Name.Value)
				constructorType = types.Any
			}
			properties["new"] = constructorType
			// Constructor signatures are always required (not optional)
		} else if prop.IsComputedProperty {
			// This is a computed property: [expr]: Type
			propType := c.resolveTypeAnnotation(prop.Type)
			if propType == nil {
				debugPrintf("// [Checker Interface P1] Failed to resolve computed property type in interface '%s'. Using Any.\n", node.Name.Value)
				propType = types.Any
			}

			// Try to extract a constant property name from the computed expression
			computedName := c.extractConstantPropertyName(prop.ComputedName)
			if computedName != "" {
				// We can resolve this to a concrete property name
				properties[computedName] = propType
				if prop.Optional {
					optionalProperties[computedName] = true
				}
				debugPrintf("// [Checker Interface P1] Interface '%s' has computed property '%s': %s\n", 
					node.Name.Value, computedName, propType.String())
			} else {
				// Dynamic computed property - treat as index signature for now
				// TODO: Better handling of dynamic computed properties
				debugPrintf("// [Checker Interface P1] Interface '%s' has dynamic computed property, treating as index signature\n", node.Name.Value)
				indexSignature := &types.IndexSignature{
					KeyType:   types.String, // Assume string keys for now
					ValueType: propType,
				}
				indexSignatures = append(indexSignatures, indexSignature)
			}
		} else if prop.Name == nil {
			// This is a call signature: (): T
			propType := c.resolveTypeAnnotation(prop.Type)
			if propType == nil {
				debugPrintf("// [Checker Interface P1] Failed to resolve call signature type in interface '%s'. Using Any.\n", node.Name.Value)
				propType = types.Any
			}
			// For now, we'll treat this as a callable interface by storing the call signature
			// In a more sophisticated implementation, we'd use CallableType
			// For simplicity, we'll just convert this to a function type and return it directly
			// TODO: Handle multiple call signatures and mixed callable/object interfaces
			debugPrintf("// [Checker Interface P1] Interface '%s' has call signature: %s\n", node.Name.Value, propType.String())

			// For now, if an interface has a call signature, we'll make it the primary type
			// This is a simplification - TypeScript allows both call signatures and properties
			if len(properties) == 0 {
				// Pure callable interface - we'll handle this after the loop
				properties["__call"] = propType
			} else {
				// Mixed interface - add as special property
				properties["__call"] = propType
			}
		} else {
			propType := c.resolveTypeAnnotation(prop.Type)
			if propType == nil {
				debugPrintf("// [Checker Interface P1] Failed to resolve type for property '%s' in interface '%s'. Using Any.\n", prop.Name.Value, node.Name.Value)
				propType = types.Any
			}

			// Check if we're overriding an inherited property
			if existingType, exists := properties[prop.Name.Value]; exists {
				// Verify that the override is compatible (for now, just log a debug message)
				debugPrintf("// [Checker Interface P1] Interface '%s' overrides property '%s' (was: %s, now: %s)\n",
					node.Name.Value, prop.Name.Value, existingType.String(), propType.String())
				// TODO: Add stricter type compatibility checking for overrides if needed
			}

			properties[prop.Name.Value] = propType

			// Track optional properties
			if prop.Optional {
				optionalProperties[prop.Name.Value] = true
			} else {
				// Explicitly mark as not optional in case it was inherited as optional
				optionalProperties[prop.Name.Value] = false
			}
		}
	}

	// 3. Create the ObjectType representing this interface
	interfaceType := &types.ObjectType{
		Properties:         properties,
		OptionalProperties: optionalProperties,
		IndexSignatures:    indexSignatures,
	}

	// 4. Define the interface as a type alias in the environment
	if !c.env.DefineTypeAlias(node.Name.Value, interfaceType) {
		debugPrintf("// [Checker Interface P1] WARNING: DefineTypeAlias failed for interface '%s'.\n", node.Name.Value)
	} else {
		debugPrintf("// [Checker Interface P1] Defined interface '%s' as type '%s' in env %p (inherited from %d interfaces)\n",
			node.Name.Value, interfaceType.String(), c.env, len(node.Extends))
	}
}

// checkGenericInterfaceDeclaration handles generic interface declarations
func (c *Checker) checkGenericInterfaceDeclaration(node *parser.InterfaceDeclaration) {
	debugPrintf("// [Checker Interface P1] Processing generic interface '%s' with %d type parameters\n", 
		node.Name.Value, len(node.TypeParameters))

	// 1. Validate type parameters
	typeParams := make([]*types.TypeParameter, len(node.TypeParameters))
	for i, param := range node.TypeParameters {
		// Create TypeParameter type
		typeParam := &types.TypeParameter{
			Name: param.Name.Value,
		}
		
		// Handle constraint if present
		if param.Constraint != nil {
			constraintType := c.resolveTypeAnnotation(param.Constraint)
			if constraintType != nil {
				typeParam.Constraint = constraintType
			}
		}
		
		// Handle default type if present
		if param.DefaultType != nil {
			defaultType := c.resolveTypeAnnotation(param.DefaultType)
			if defaultType != nil {
				typeParam.Default = defaultType
				
				// Validate that default type satisfies constraint if both are present
				if typeParam.Constraint != nil && !types.IsAssignable(defaultType, typeParam.Constraint) {
					c.addError(param.DefaultType, fmt.Sprintf("default type '%s' does not satisfy constraint '%s'", defaultType.String(), typeParam.Constraint.String()))
				}
			}
		}
		
		typeParams[i] = typeParam
	}

	// 2. Create the body ObjectType with TypeParameterType references
	// This is a template that will be instantiated with concrete types later
	
	// Create a new environment with type parameters available as TypeParameterType
	genericEnv := NewEnclosedEnvironment(c.env)
	for _, typeParam := range typeParams {
		paramType := &types.TypeParameterType{
			Parameter: typeParam,
		}
		genericEnv.DefineTypeAlias(typeParam.Name, paramType)
	}
	
	// Save current environment and switch to generic environment
	savedEnv := c.env
	c.env = genericEnv
	
	// Build the ObjectType body with TypeParameterType references
	properties := make(map[string]types.Type)
	optionalProperties := make(map[string]bool)
	var indexSignatures []*types.IndexSignature

	// Handle extends clause with generic environment
	for _, extendedInterfaceExpr := range node.Extends {
		// Resolve the extended interface type expression in the generic environment first
		extendedType := c.resolveTypeAnnotation(extendedInterfaceExpr)
		if extendedType == nil {
			// If resolution fails in generic environment, restore original and try again
			savedEnvTemp := c.env
			c.env = savedEnv
			extendedType = c.resolveTypeAnnotation(extendedInterfaceExpr)
			c.env = savedEnvTemp
		}
		if extendedType == nil {
			continue // Skip unresolved extended interfaces
		}

		if extendedObjectType, ok := extendedType.(*types.ObjectType); ok {
			for propName, propType := range extendedObjectType.Properties {
				properties[propName] = propType
				if extendedObjectType.OptionalProperties != nil && extendedObjectType.OptionalProperties[propName] {
					optionalProperties[propName] = true
				}
			}
		}
	}

	// Process interface properties with TypeParameterType references
	for _, prop := range node.Properties {
		if prop.IsIndexSignature {
			// Handle index signature: [key: KeyType]: ValueType
			keyType := c.resolveTypeAnnotation(prop.KeyType)
			if keyType == nil {
				keyType = types.String
			}
			
			valueType := c.resolveTypeAnnotation(prop.ValueType)
			if valueType == nil {
				valueType = types.Any
			}
			
			indexSignature := &types.IndexSignature{
				KeyType:   keyType,
				ValueType: valueType,
			}
			indexSignatures = append(indexSignatures, indexSignature)
		} else if prop.IsConstructorSignature {
			constructorType := c.resolveTypeAnnotation(prop.Type)
			if constructorType == nil {
				constructorType = types.Any
			}
			properties["new"] = constructorType
		} else if prop.Name == nil {
			propType := c.resolveTypeAnnotation(prop.Type)
			if propType == nil {
				propType = types.Any
			}
			properties["__call"] = propType
		} else {
			propType := c.resolveTypeAnnotation(prop.Type)
			if propType == nil {
				propType = types.Any
			}
			properties[prop.Name.Value] = propType
			if prop.Optional {
				optionalProperties[prop.Name.Value] = true
			}
		}
	}

	// Restore environment
	c.env = savedEnv

	bodyType := &types.ObjectType{
		Properties:         properties,
		OptionalProperties: optionalProperties,
		IndexSignatures:    indexSignatures,
	}

	// 3. Create the GenericType
	genericType := &types.GenericType{
		Name:           node.Name.Value,
		TypeParameters: typeParams,
		Body:           bodyType,
	}

	// 4. Define in environment
	if !c.env.DefineTypeAlias(node.Name.Value, genericType) {
		debugPrintf("// [Checker Interface P1] WARNING: DefineTypeAlias failed for generic interface '%s'.\n", node.Name.Value)
	} else {
		debugPrintf("// [Checker Interface P1] Defined generic interface '%s' with %d type parameters in env %p\n",
			node.Name.Value, len(typeParams), c.env)
	}
}

func (c *Checker) checkSwitchStatement(node *parser.SwitchStatement) {
	// 1. Visit the switch expression
	c.visit(node.Expression)
	switchExprType := node.Expression.GetComputedType()
	if switchExprType == nil {
		switchExprType = types.Any // Default to Any if expression check failed
	}
	widenedSwitchExprType := types.GetWidenedType(switchExprType)

	// 2. Visit cases
	for _, caseClause := range node.Cases {
		if caseClause.Condition != nil {
			// Visit case condition
			c.visit(caseClause.Condition)
			caseCondType := caseClause.Condition.GetComputedType()
			if caseCondType == nil {
				caseCondType = types.Any // Default to Any on error
			}
			widenedCaseCondType := types.GetWidenedType(caseCondType)

			// --- Basic Comparability Check --- (Simplified for now)
			// Allow comparison if either is Any/Unknown or if they are the same primitive type.
			// This is not perfect for === semantics but catches basic errors.
			isAny := widenedSwitchExprType == types.Any || widenedCaseCondType == types.Any
			isUnknown := widenedSwitchExprType == types.Unknown || widenedCaseCondType == types.Unknown
			// Check for function types (unified ObjectType with call signatures)
			switchObjCallable := false
			caseObjCallable := false
			if switchObj, ok := widenedSwitchExprType.(*types.ObjectType); ok {
				switchObjCallable = switchObj.IsCallable()
			}
			if caseObj, ok := widenedCaseCondType.(*types.ObjectType); ok {
				caseObjCallable = caseObj.IsCallable()
			}
			isSwitchFunc := switchObjCallable
			isCaseFunc := caseObjCallable
			_, isSwitchArray := widenedSwitchExprType.(*types.ArrayType)
			_, isCaseArray := widenedCaseCondType.(*types.ArrayType)

			incompatible := false
			if (isSwitchFunc != isCaseFunc) && !isAny && !isUnknown {
				incompatible = true // Can't compare func and non-func (unless any/unknown)
			}
			if (isSwitchArray != isCaseArray) && !isAny && !isUnknown {
				// Maybe allow comparing array to any/null/undefined? Needs refinement.
				// For now, treat array vs non-array as incompatible unless any/unknown involved.
				incompatible = true
			}
			// Add more checks? e.g., number vs string? Current VM might coerce.
			// Strict equality usually doesn't coerce, so maybe check primitives match?

			if incompatible {
				c.addError(caseClause.Condition, fmt.Sprintf("this case expression type (%s) is not comparable to the switch expression type (%s)", widenedCaseCondType, widenedSwitchExprType))
			}
			// --- End Comparability Check ---
		}

		// Visit case body (BlockStatement, handles its own scope)
		c.visit(caseClause.Body)
	}

	// Switch statements don't produce a value themselves
	// node.SetComputedType(types.Void) // Remove this line
}

func (c *Checker) checkForStatement(node *parser.ForStatement) {
	// --- UPDATED: Handle For Statement Scope ---
	// 1. Create a new enclosed environment for the loop
	originalEnv := c.env
	loopEnv := NewEnclosedEnvironment(originalEnv)
	c.env = loopEnv
	debugPrintf("// [Checker ForStmt] Created loop scope %p (outer: %p)\n", loopEnv, originalEnv)

	// 2. Visit parts within the loop scope
	c.visit(node.Initializer)
	c.visit(node.Condition)
	c.visit(node.Update)
	c.visit(node.Body)

	// 3. Restore the outer environment
	c.env = originalEnv
	debugPrintf("// [Checker ForStmt] Restored outer scope %p (from %p)\n", originalEnv, loopEnv)

}

func (c *Checker) checkForOfStatement(node *parser.ForOfStatement) {
	// Handle for...of statement scope
	if node == nil {
		c.addError(nil, "nil ForOfStatement node")
		return
	}

	originalEnv := c.env
	loopEnv := NewEnclosedEnvironment(originalEnv)
	c.env = loopEnv
	debugPrintf("// [Checker ForOfStmt] Created loop scope %p (outer: %p)\n", loopEnv, originalEnv)

	// Visit the iterable first to determine its type
	if node.Iterable != nil {
		c.visit(node.Iterable)
		iterableType := node.Iterable.GetComputedType()
		if iterableType == nil {
			iterableType = types.Any
		}

		// Determine the element type from the iterable
		var elementType types.Type
		if arrayType, ok := iterableType.(*types.ArrayType); ok {
			elementType = arrayType.ElementType
		} else if iterableType == types.String || types.IsAssignable(iterableType, types.String) {
			// String iteration yields individual characters (strings)
			// This handles both the general string type and string literal types
			elementType = types.String
		} else if iterableType == types.Any {
			elementType = types.Any
		} else if c.isGeneratorType(iterableType) {
			// Special handling for Generator types - they are iterable
			elementType = types.Any // Safe fallback for generator elements
		} else {
			// Special case: if the iterable comes from a generator function call, assume it's iterable
			// This handles cases where generator functions haven't been fully resolved yet in multi-pass checking
			if callExpr, ok := node.Iterable.(*parser.CallExpression); ok {
				if ident, ok := callExpr.Function.(*parser.Identifier); ok {
					if c.generatorFunctions[ident.Value] {
						elementType = types.Any // Safe fallback for generator elements
						goto handleVariable
					}
				}
			}
			
			// Check if the type is assignable to Iterable<any>
			if iterableGeneric, found, _ := c.env.Resolve("Iterable"); found {
				if genericType, ok := iterableGeneric.(*types.GenericType); ok {
					// Create Iterable<any> to check assignability
					iterableAny := &types.InstantiatedType{
						Generic:       genericType,
						TypeArguments: []types.Type{types.Any},
					}
					
					if types.IsAssignable(iterableType, iterableAny) {
						// It's iterable, but we can't easily extract the element type
						// For now, use Any as a safe fallback
						elementType = types.Any
					} else {
						// Not iterable
						c.addError(node.Iterable, fmt.Sprintf("type '%s' is not iterable", iterableType.String()))
						elementType = types.Any
					}
				} else {
					// Iterable type exists but isn't a generic - something's wrong
					c.addError(node.Iterable, fmt.Sprintf("type '%s' is not iterable", iterableType.String()))
					elementType = types.Any
				}
			} else {
				// No Iterable type found - fallback to old behavior
				c.addError(node.Iterable, fmt.Sprintf("type '%s' is not iterable", iterableType.String()))
				elementType = types.Any
			}
		}

handleVariable:
		// Handle the variable declaration/assignment
		if node.Variable != nil {
			if letStmt, ok := node.Variable.(*parser.LetStatement); ok {
				// Define the loop variable with the element type
				if letStmt.Name != nil {
					c.env.Define(letStmt.Name.Value, elementType, false)
					letStmt.ComputedType = elementType
					letStmt.Name.SetComputedType(elementType)
				}
			} else if constStmt, ok := node.Variable.(*parser.ConstStatement); ok {
				// Define the loop variable with the element type
				if constStmt.Name != nil {
					c.env.Define(constStmt.Name.Value, elementType, true) // const = true
					constStmt.ComputedType = elementType
					constStmt.Name.SetComputedType(elementType)
				}
			} else if exprStmt, ok := node.Variable.(*parser.ExpressionStatement); ok {
				// This is an existing variable being assigned to
				if exprStmt.Expression != nil {
					if ident, ok := exprStmt.Expression.(*parser.Identifier); ok {
						// Check if the variable exists and is assignable
						varType, _, exists := c.env.Resolve(ident.Value)
						if !exists {
							c.addError(ident, fmt.Sprintf("undefined variable '%s'", ident.Value))
						} else if !types.IsAssignable(elementType, varType) {
							c.addError(ident, fmt.Sprintf("cannot assign element type '%s' to variable type '%s'", elementType.String(), varType.String()))
						}
						ident.SetComputedType(elementType)
					}
				}
			}
		}

		// Visit the body
		if node.Body != nil {
			c.visit(node.Body)
		}
	} else {
		c.addError(node, "for...of statement missing iterable expression")
	}

	// Restore the outer environment
	c.env = originalEnv
	debugPrintf("// [Checker ForOfStmt] Restored outer scope %p (from %p)\n", originalEnv, loopEnv)

}

func (c *Checker) checkForInStatement(node *parser.ForInStatement) {
	// Handle for...in statement scope
	if node == nil {
		c.addError(nil, "nil ForInStatement node")
		return
	}

	originalEnv := c.env
	loopEnv := NewEnclosedEnvironment(originalEnv)
	c.env = loopEnv
	debugPrintf("// [Checker ForInStmt] Created loop scope %p (outer: %p)\n", loopEnv, originalEnv)

	// Visit the object first to determine its type
	if node.Object != nil {
		c.visit(node.Object)
		objectType := node.Object.GetComputedType()
		if objectType == nil {
			objectType = types.Any
		}

		// For...in loops always yield property names as strings
		// This is true for objects, arrays, and other enumerable types
		elementType := types.String

		// Validate that the object type is enumerable
		// In JavaScript/TypeScript, most object types are enumerable
		switch objectType {
		case types.Null, types.Undefined:
			c.addError(node.Object, fmt.Sprintf("cannot iterate over '%s'", objectType.String()))
		default:
			// Most types are enumerable in for...in (objects, arrays, etc.)
			// Even primitives like numbers and strings are allowed (though they may have no enumerable properties)
		}

		// Handle the variable declaration/assignment
		if node.Variable != nil {
			if letStmt, ok := node.Variable.(*parser.LetStatement); ok {
				// Define the loop variable with string type (property names)
				if letStmt.Name != nil {
					c.env.Define(letStmt.Name.Value, elementType, false)
					letStmt.ComputedType = elementType
					letStmt.Name.SetComputedType(elementType)
				}
			} else if constStmt, ok := node.Variable.(*parser.ConstStatement); ok {
				// Define the loop variable with string type (property names)
				if constStmt.Name != nil {
					c.env.Define(constStmt.Name.Value, elementType, true) // const = true
					constStmt.ComputedType = elementType
					constStmt.Name.SetComputedType(elementType)
				}
			} else if exprStmt, ok := node.Variable.(*parser.ExpressionStatement); ok {
				// This is an existing variable being assigned to
				if exprStmt.Expression != nil {
					if ident, ok := exprStmt.Expression.(*parser.Identifier); ok {
						// Check if the variable exists and is assignable to string
						varType, _, exists := c.env.Resolve(ident.Value)
						if !exists {
							c.addError(ident, fmt.Sprintf("undefined variable '%s'", ident.Value))
						} else if !types.IsAssignable(elementType, varType) {
							c.addError(ident, fmt.Sprintf("cannot assign property name type '%s' to variable type '%s'", elementType.String(), varType.String()))
						}
						ident.SetComputedType(elementType)
					}
				}
			}
		}

		// Visit the body
		if node.Body != nil {
			c.visit(node.Body)
		}
	} else {
		c.addError(node, "for...in statement missing object expression")
	}

	// Restore the outer environment
	c.env = originalEnv
	debugPrintf("// [Checker ForInStmt] Restored outer scope %p (from %p)\n", originalEnv, loopEnv)
}

// extractConstantPropertyName tries to extract a constant property name from a computed property expression
func (c *Checker) extractConstantPropertyName(expr parser.Expression) string {
	switch e := expr.(type) {
	case *parser.StringLiteral:
		return e.Value
	case *parser.NumberLiteral:
		return fmt.Sprintf("%v", e.Value)
	case *parser.Identifier:
		// For now, we can't resolve identifiers to their constant values during interface checking
		// TODO: Add constant folding support
		return ""
	default:
		return ""
	}
}

// checkWithStatement performs type checking for 'with' statements
func (c *Checker) checkWithStatement(node *parser.WithStatement) {
	if node == nil {
		c.addError(nil, "nil WithStatement node")
		return
	}

	// Check the expression (object to extend scope with)
	var withObj WithObject
	if node.Expression != nil {
		c.visit(node.Expression)
		
		// The expression should be an object type for proper 'with' semantics
		exprType := node.Expression.GetComputedType()
		if exprType == nil {
			exprType = types.Any
		}
		
		// Create with object and extract known properties
		properties := c.extractPropertiesFromType(exprType)
		withObj = WithObject{
			ExprType:   exprType,
			Properties: properties,
		}
		
		debugPrintf("// [Checker WithStmt] Expression type: %s, extracted %d properties\n", exprType.String(), len(properties))
		for propName, propType := range properties {
			debugPrintf("// [Checker WithStmt] Property '%s': %s\n", propName, propType.String())
		}
		
		// Push the with object onto the environment stack
		c.env.PushWithObject(withObj)
	}

	// Check the body with the with object in scope
	if node.Body != nil {
		c.visit(node.Body)
	}
	
	// Pop the with object when done
	if node.Expression != nil {
		c.env.PopWithObject()
	}
}

// extractPropertiesFromType extracts known properties from a type for with statement scope resolution
func (c *Checker) extractPropertiesFromType(typ types.Type) map[string]types.Type {
	properties := make(map[string]types.Type)
	
	if typ == nil {
		return properties
	}
	
	switch t := typ.(type) {
	case *types.ObjectType:
		// Copy all properties from the object type
		for propName, propType := range t.Properties {
			properties[propName] = propType
		}
	case *types.Primitive:
		// For 'any' type, we can't know properties at compile time
		if t == types.Any {
			// The compiler will need to emit property access for all unresolved identifiers
			// Return empty map - properties will be resolved at runtime
		}
	default:
		// For other types (primitives, etc.), no known properties
		// In JavaScript, you can use 'with' on any object, but primitives
		// don't have enumerable own properties
	}
	
	return properties
}

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

	// 2. Resolve the RHS type using the CURRENT (global) environment
	// This allows aliases to reference previously defined aliases in the same pass
	aliasedType := c.resolveTypeAnnotation(node.Type) // Uses c.env (globalEnv)
	if aliasedType == nil {
		debugPrintf("// [Checker TypeAlias P1] Failed to resolve type for alias '%s'. Defining as Any.\n", node.Name.Value)
		if !c.env.DefineTypeAlias(node.Name.Value, types.Any) {
			debugPrintf("// [Checker TypeAlias P1] WARNING: DefineTypeAlias failed for '%s' (as Any).\n", node.Name.Value)
		}
		return
	}

	// 3. Define the alias in the CURRENT (global) environment
	if !c.env.DefineTypeAlias(node.Name.Value, aliasedType) {
		debugPrintf("// [Checker TypeAlias P1] WARNING: DefineTypeAlias failed for '%s'.\n", node.Name.Value)
	} else {
		debugPrintf("// [Checker TypeAlias P1] Defined alias '%s' as type '%s' in env %p\n", node.Name.Value, aliasedType.String(), c.env)
	}
	// No need to set computed type on the TypeAliasStatement node itself
}

// --- NEW: Interface Declaration Check ---

func (c *Checker) checkInterfaceDeclaration(node *parser.InterfaceDeclaration) {
	// Called during Pass 1 for interface declarations

	// 1. Check if already defined
	if _, exists := c.env.ResolveType(node.Name.Value); exists {
		debugPrintf("// [Checker Interface P1] Interface '%s' already defined? Skipping.\n", node.Name.Value)
		return
	}

	// 2. Build the ObjectType from interface properties, including inheritance
	properties := make(map[string]types.Type)
	optionalProperties := make(map[string]bool)

	// First, inherit properties from extended interfaces
	for _, extendedInterfaceName := range node.Extends {
		extendedType, exists := c.env.ResolveType(extendedInterfaceName.Value)
		if !exists {
			c.addError(extendedInterfaceName, fmt.Sprintf("extended interface '%s' is not defined", extendedInterfaceName.Value))
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
			debugPrintf("// [Checker Interface P1] Interface '%s' inherited %d properties from '%s'\n",
				node.Name.Value, len(extendedObjectType.Properties), extendedInterfaceName.Value)
		} else {
			c.addError(extendedInterfaceName, fmt.Sprintf("'%s' is not an interface, cannot extend", extendedInterfaceName.Value))
		}
	}

	// Then, add/override properties from this interface's declaration
	for _, prop := range node.Properties {
		if prop.IsConstructorSignature {
			// For constructor signatures, add them as a special "new" property
			// This allows the interface to describe both instance properties and constructor behavior
			constructorType := c.resolveTypeAnnotation(prop.Type)
			if constructorType == nil {
				debugPrintf("// [Checker Interface P1] Failed to resolve constructor type in interface '%s'. Using Any.\n", node.Name.Value)
				constructorType = types.Any
			}
			properties["new"] = constructorType
			// Constructor signatures are always required (not optional)
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
	}

	// 4. Define the interface as a type alias in the environment
	if !c.env.DefineTypeAlias(node.Name.Value, interfaceType) {
		debugPrintf("// [Checker Interface P1] WARNING: DefineTypeAlias failed for interface '%s'.\n", node.Name.Value)
	} else {
		debugPrintf("// [Checker Interface P1] Defined interface '%s' as type '%s' in env %p (inherited from %d interfaces)\n",
			node.Name.Value, interfaceType.String(), c.env, len(node.Extends))
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
			_, isSwitchFunc := widenedSwitchExprType.(*types.FunctionType)
			_, isCaseFunc := widenedCaseCondType.(*types.FunctionType)
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
		} else {
			// For now, assume any other type is not iterable
			c.addError(node.Iterable, fmt.Sprintf("type '%s' is not iterable", iterableType.String()))
			elementType = types.Any
		}

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

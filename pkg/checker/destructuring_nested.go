package checker

import (
	"fmt"
	"paserati/pkg/parser"
	"paserati/pkg/types"
)

// checkDestructuringTarget handles recursive type checking for destructuring targets
func (c *Checker) checkDestructuringTarget(target parser.Expression, expectedType types.Type, context interface{}) {
	switch targetNode := target.(type) {
	case *parser.Identifier:
		c.checkIdentifierTarget(targetNode, expectedType)
	case *parser.ArrayLiteral:
		c.checkNestedArrayTarget(targetNode, expectedType, context)
	case *parser.ObjectLiteral:
		c.checkNestedObjectTarget(targetNode, expectedType, context)
	case *parser.UndefinedLiteral:
		// Elision in destructuring - no type checking needed, just skip this element
		return
	default:
		c.addError(target, fmt.Sprintf("invalid destructuring target type: %T", target))
	}
}

// checkDestructuringTargetForProperty handles recursive type checking for property destructuring targets
func (c *Checker) checkDestructuringTargetForProperty(target parser.Expression, expectedType types.Type, propName string) {
	switch targetNode := target.(type) {
	case *parser.Identifier:
		c.checkIdentifierTarget(targetNode, expectedType)
	case *parser.ArrayLiteral:
		c.checkNestedArrayTarget(targetNode, expectedType, propName)
	case *parser.ObjectLiteral:
		c.checkNestedObjectTarget(targetNode, expectedType, propName)
	case *parser.UndefinedLiteral:
		// Elision in destructuring - no type checking needed, just skip this element
		return
	default:
		c.addError(target, fmt.Sprintf("invalid destructuring target type: %T", target))
	}
}

// checkIdentifierTarget handles type checking for identifier targets in destructuring
func (c *Checker) checkIdentifierTarget(target *parser.Identifier, expectedType types.Type) {
	// For union types, try to select the most appropriate type
	finalType := expectedType
	if unionType, ok := expectedType.(*types.UnionType); ok {
		// For destructuring, prefer non-array types for simple identifiers
		// This handles cases like [a, [b, c]] where b and c should be numbers, not number | number[]
		for _, memberType := range unionType.Types {
			// Prefer primitive types over array types for simple variable assignment
			if memberType == types.Number || memberType == types.String || memberType == types.Boolean {
				finalType = memberType
				break
			}
		}
	}
	
	// Set the computed type for the identifier
	target.SetComputedType(finalType)
	
	// Update the environment with the new variable binding
	c.env.Update(target.Value, finalType)
}

// checkNestedArrayTarget handles type checking for nested array destructuring targets
func (c *Checker) checkNestedArrayTarget(arrayTarget *parser.ArrayLiteral, expectedType types.Type, context interface{}) {
	// Validate that expectedType is array-like
	widenedType := types.GetWidenedType(expectedType)
	var elementType types.Type
	
	if arrayType, ok := widenedType.(*types.ArrayType); ok {
		elementType = arrayType.ElementType
	} else if tupleType, ok := widenedType.(*types.TupleType); ok {
		// For tuple types, check each element with its specific type
		for i, element := range arrayTarget.Elements {
			var elemType types.Type
			if i < len(tupleType.ElementTypes) {
				elemType = tupleType.ElementTypes[i]
			} else {
				elemType = types.Undefined
			}
			c.checkDestructuringTarget(element, elemType, i)
		}
		return
	} else if unionType, ok := expectedType.(*types.UnionType); ok {
		// Check if any type in the union is array-like
		var arrayLikeType types.Type
		for _, unionMember := range unionType.Types {
			if arrayType, ok := unionMember.(*types.ArrayType); ok {
				arrayLikeType = arrayType
				break
			} else if _, ok := unionMember.(*types.TupleType); ok {
				arrayLikeType = unionMember
				break
			}
		}
		
		if arrayLikeType != nil {
			// Recursively check with the array-like type from the union
			c.checkNestedArrayTarget(arrayTarget, arrayLikeType, context)
			return
		}
		
		// If no array-like type found in union, fallback to Any
		elementType = types.Any
	} else if widenedType == types.Any {
		elementType = types.Any
	} else {
		c.addError(arrayTarget, fmt.Sprintf("cannot destructure array pattern from non-array type '%s'", expectedType.String()))
		elementType = types.Any
	}
	
	// For regular array types, check each element with the same element type
	for i, element := range arrayTarget.Elements {
		c.checkDestructuringTarget(element, elementType, i)
	}
}

// checkNestedObjectTarget handles type checking for nested object destructuring targets
func (c *Checker) checkNestedObjectTarget(objectTarget *parser.ObjectLiteral, expectedType types.Type, context interface{}) {
	// Validate that expectedType is object-like
	widenedType := types.GetWidenedType(expectedType)

	if widenedType != types.Any {
		objType, ok := expectedType.(*types.ObjectType)
		if !ok {
			// Arrays can also be destructured as objects
			if _, ok := expectedType.(*types.ArrayType); ok {
				// Allow array destructuring as object (e.g., {0: x, 1: y} from array)
				// Properties will be checked as any since we can't statically know array indices
			} else if unionType, ok := expectedType.(*types.UnionType); ok {
				// Try to handle union types
				// Look for an object type or array type in the union
				for _, memberType := range unionType.Types {
					if objectType, ok := memberType.(*types.ObjectType); ok {
						objType = objectType
						break
					}
					if _, ok := memberType.(*types.ArrayType); ok {
						// Found array type, allow it
						break
					}
				}

				if objType == nil {
					c.addError(objectTarget, fmt.Sprintf("cannot destructure object pattern from non-object type '%s'", expectedType.String()))
					return
				}
			} else {
				c.addError(objectTarget, fmt.Sprintf("cannot destructure object pattern from non-object type '%s'", expectedType.String()))
				return
			}
		}
		
		// Check each property in the nested object pattern
		for _, prop := range objectTarget.Properties {
			// Get property name from key (can be identifier or number)
			var propName string
			if keyIdent, ok := prop.Key.(*parser.Identifier); ok {
				propName = keyIdent.Value
			} else if numLit, ok := prop.Key.(*parser.NumberLiteral); ok {
				propName = numLit.Token.Literal
			} else {
				c.addError(prop.Key, "object destructuring key must be an identifier or number")
				continue
			}

			var propType types.Type = types.Undefined

			if objType != nil {
				if foundType, exists := objType.Properties[propName]; exists {
					propType = foundType
				}
			} else if _, ok := expectedType.(*types.ArrayType); ok {
				// For arrays destructured as objects, use element type for numeric keys, any for others
				if arrType, ok := expectedType.(*types.ArrayType); ok {
					propType = arrType.ElementType
				}
			}

			c.checkDestructuringTargetForProperty(prop.Value, propType, propName)
		}
	} else {
		// For Any type, all nested targets get Any type
		for _, prop := range objectTarget.Properties {
			c.checkDestructuringTargetForProperty(prop.Value, types.Any, "")
		}
	}
}

// checkDestructuringTargetForDeclaration handles type checking and environment definition for destructuring targets in declarations
func (c *Checker) checkDestructuringTargetForDeclaration(target parser.Expression, expectedType types.Type, isConst bool) {
	switch targetNode := target.(type) {
	case *parser.Identifier:
		// Simple identifier target - define in environment with refined type
		finalType := expectedType
		if unionType, ok := expectedType.(*types.UnionType); ok {
			// For destructuring declarations, prefer non-array types for simple identifiers
			// This handles cases like let [a, [b, c]] where b and c should be numbers, not number | number[]
			for _, memberType := range unionType.Types {
				// Prefer primitive types over array types for simple variable assignment
				if memberType == types.Number || memberType == types.String || memberType == types.Boolean {
					finalType = memberType
					break
				}
			}
		}
		
		if !c.env.Define(targetNode.Value, finalType, isConst) {
			c.addError(targetNode, fmt.Sprintf("identifier '%s' already declared", targetNode.Value))
		}
		targetNode.SetComputedType(finalType)
	case *parser.ArrayLiteral:
		// Nested array destructuring declaration
		c.checkNestedArrayTargetForDeclaration(targetNode, expectedType, isConst)
	case *parser.ObjectLiteral:
		// Nested object destructuring declaration
		c.checkNestedObjectTargetForDeclaration(targetNode, expectedType, isConst)
	case *parser.UndefinedLiteral:
		// Elision in destructuring - no type checking needed, just skip this element
		return
	default:
		c.addError(target, fmt.Sprintf("invalid destructuring target type: %T", target))
	}
}

// checkNestedArrayTargetForDeclaration handles type checking for nested array destructuring in declarations
func (c *Checker) checkNestedArrayTargetForDeclaration(arrayTarget *parser.ArrayLiteral, expectedType types.Type, isConst bool) {
	// Validate that expectedType is array-like
	widenedType := types.GetWidenedType(expectedType)
	var elementType types.Type
	
	if arrayType, ok := widenedType.(*types.ArrayType); ok {
		elementType = arrayType.ElementType
	} else if tupleType, ok := widenedType.(*types.TupleType); ok {
		// For tuple types, check each element with its specific type
		for i, element := range arrayTarget.Elements {
			var elemType types.Type
			if i < len(tupleType.ElementTypes) {
				elemType = tupleType.ElementTypes[i]
			} else {
				elemType = types.Undefined
			}
			c.checkDestructuringTargetForDeclaration(element, elemType, isConst)
		}
		return
	} else if unionType, ok := expectedType.(*types.UnionType); ok {
		// Check if any type in the union is array-like
		var arrayLikeType types.Type
		for _, unionMember := range unionType.Types {
			if arrayType, ok := unionMember.(*types.ArrayType); ok {
				arrayLikeType = arrayType
				break
			} else if _, ok := unionMember.(*types.TupleType); ok {
				arrayLikeType = unionMember
				break
			}
		}
		
		if arrayLikeType != nil {
			// Recursively check with the array-like type from the union
			c.checkNestedArrayTargetForDeclaration(arrayTarget, arrayLikeType, isConst)
			return
		}
		
		// If no array-like type found in union, fallback to Any
		elementType = types.Any
	} else if widenedType == types.Any {
		elementType = types.Any
	} else {
		c.addError(arrayTarget, fmt.Sprintf("cannot destructure array pattern from non-array type '%s'", expectedType.String()))
		elementType = types.Any
	}
	
	// For regular array types, check each element with the same element type
	for _, element := range arrayTarget.Elements {
		c.checkDestructuringTargetForDeclaration(element, elementType, isConst)
	}
}

// checkNestedObjectTargetForDeclaration handles type checking for nested object destructuring in declarations
func (c *Checker) checkNestedObjectTargetForDeclaration(objectTarget *parser.ObjectLiteral, expectedType types.Type, isConst bool) {
	// Validate that expectedType is object-like
	widenedType := types.GetWidenedType(expectedType)

	if widenedType != types.Any {
		objType, ok := expectedType.(*types.ObjectType)
		if !ok {
			// Arrays can also be destructured as objects
			if _, ok := expectedType.(*types.ArrayType); ok {
				// Allow array destructuring as object (e.g., {0: x, 1: y} from array)
				// Properties will be checked as any since we can't statically know array indices
			} else if unionType, ok := expectedType.(*types.UnionType); ok {
				// Try to handle union types
				// Look for an object type or array type in the union
				for _, memberType := range unionType.Types {
					if objectType, ok := memberType.(*types.ObjectType); ok {
						objType = objectType
						break
					}
					if _, ok := memberType.(*types.ArrayType); ok {
						// Found array type, allow it
						break
					}
				}

				if objType == nil {
					c.addError(objectTarget, fmt.Sprintf("cannot destructure object pattern from non-object type '%s'", expectedType.String()))
					return
				}
			} else {
				c.addError(objectTarget, fmt.Sprintf("cannot destructure object pattern from non-object type '%s'", expectedType.String()))
				return
			}
		}
		
		// Check each property in the nested object pattern
		for _, prop := range objectTarget.Properties {
			// Get property name from key (can be identifier or number)
			var propName string
			if keyIdent, ok := prop.Key.(*parser.Identifier); ok {
				propName = keyIdent.Value
			} else if numLit, ok := prop.Key.(*parser.NumberLiteral); ok {
				propName = numLit.Token.Literal
			} else {
				c.addError(prop.Key, "object destructuring key must be an identifier or number")
				continue
			}

			var propType types.Type = types.Undefined

			if objType != nil {
				if foundType, exists := objType.Properties[propName]; exists {
					propType = foundType
				}
			} else if _, ok := expectedType.(*types.ArrayType); ok {
				// For arrays destructured as objects, use element type for numeric keys, any for others
				if arrType, ok := expectedType.(*types.ArrayType); ok {
					propType = arrType.ElementType
				}
			}

			c.checkDestructuringTargetForDeclaration(prop.Value, propType, isConst)
		}
	} else {
		// For Any type, all nested targets get Any type
		for _, prop := range objectTarget.Properties {
			c.checkDestructuringTargetForDeclaration(prop.Value, types.Any, isConst)
		}
	}
}
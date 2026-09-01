package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// nestedObjectPatternRestType is the type a rest binding inside a nested object
// pattern receives: an object of the source's properties minus the ones the
// pattern extracted by name. Mirrors the top-level RestProperty case in
// checkObjectDestructuringDeclaration.
//
// A nested pattern is a raw ObjectLiteral, so its rest arrives as a
// SpreadElement in ObjectProperty.Key rather than in a RestProperty field -
// which is why it reaches the key check rather than the target walk.
func nestedObjectPatternRestType(source types.Type, pattern *parser.ObjectLiteral) types.Type {
	if types.GetWidenedType(source) == types.Any {
		return types.Any
	}
	objType, ok := source.(*types.ObjectType)
	if !ok {
		return &types.ObjectType{Properties: make(map[string]types.Type)}
	}
	extracted := make(map[string]struct{})
	for _, prop := range pattern.Properties {
		if prop == nil {
			continue
		}
		if keyIdent, ok := prop.Key.(*parser.Identifier); ok {
			extracted[keyIdent.Value] = struct{}{}
		} else if numLit, ok := prop.Key.(*parser.NumberLiteral); ok {
			extracted[numLit.Token.Literal] = struct{}{}
		}
		// Computed keys and the rest element itself contribute nothing.
	}
	remaining := make(map[string]types.Type)
	for name, t := range objType.Properties {
		if _, wasExtracted := extracted[name]; !wasExtracted {
			remaining[name] = t
		}
	}
	return &types.ObjectType{Properties: remaining}
}

// unwrapNestedPatternTarget peels the wrappers a *nested* destructuring target
// can carry, and adjusts the type the binding inside should receive.
//
// A pattern is represented two ways depending on depth. At the top level of a
// declaration a default lives in DestructuringProperty.Default /
// DestructuringElement.Default and a rest in DestructuringElement.IsRest, with
// Target holding the binding. Nested one level down, Target is a raw
// ObjectLiteral/ArrayLiteral, so a default appears as an AssignmentExpression
// and a rest as a SpreadElement, with the binding inside the wrapper. The
// target walks knew Identifier/ObjectLiteral/ArrayLiteral but neither wrapper,
// so `const {h: {i = 7}} = o` and `const [[j, ...k]] = o` were rejected with
// "invalid destructuring target type: *parser.AssignmentExpression/SpreadElement"
// even though the runtime binds them correctly.
//
// The type adjustments mirror the top-level cases: a default replaces an
// Undefined/Unknown type with its own widened type, and a rest binding gets an
// array of the element type. Returns the target unchanged when it carries
// neither wrapper.
func (c *Checker) unwrapNestedPatternTarget(target parser.Expression, expectedType types.Type) (parser.Expression, types.Type) {
	switch t := target.(type) {
	case *parser.AssignmentExpression:
		// Nested default: `{h: {i = 7}}` / `[[m = 4]]`.
		if t.Value != nil {
			c.visit(t.Value)
			defaultType := t.Value.GetComputedType()
			if defaultType == nil {
				defaultType = types.Any
			}
			if expectedType == types.Undefined || expectedType == types.Unknown {
				expectedType = types.GetWidenedType(defaultType)
			}
		}
		return t.Left, expectedType
	case *parser.SpreadElement:
		// Nested rest: `[[x, ...ys]]`. The caller passes the element type, so an
		// array of it is right for a plain array or Any source. A tuple source
		// needs the union of the *remaining* element types instead, which only
		// the array walk knows the position for - it calls restElementType
		// directly and bypasses this.
		return t.Argument, &types.ArrayType{ElementType: expectedType}
	}
	return target, expectedType
}

// restElementType is the type a rest binding receives when destructuring source
// at position index: an array of the remaining element types. Mirrors the
// top-level IsRest case in checkArrayDestructuringDeclaration.
func restElementType(source types.Type, index int) types.Type {
	switch t := source.(type) {
	case *types.ArrayType:
		return &types.ArrayType{ElementType: t.ElementType}
	case *types.TupleType:
		if index >= len(t.ElementTypes) {
			return &types.ArrayType{ElementType: types.Never}
		}
		remaining := t.ElementTypes[index:]
		if len(remaining) == 1 {
			return &types.ArrayType{ElementType: remaining[0]}
		}
		return &types.ArrayType{ElementType: &types.UnionType{Types: remaining}}
	default:
		return &types.ArrayType{ElementType: types.Any}
	}
}

// checkDestructuringTarget handles recursive type checking for destructuring targets
func (c *Checker) checkDestructuringTarget(target parser.Expression, expectedType types.Type, context interface{}) {
	switch targetNode := target.(type) {
	case *parser.Identifier:
		c.checkIdentifierTarget(targetNode, expectedType)
	case *parser.ArrayLiteral:
		c.checkNestedArrayTarget(targetNode, expectedType, context)
	case *parser.ObjectLiteral:
		c.checkNestedObjectTarget(targetNode, expectedType, context)
	case *parser.ArrayParameterPattern:
		// Handle nested array parameter patterns (from function parameters)
		c.checkNestedArrayParameterPattern(targetNode, expectedType, context)
	case *parser.ObjectParameterPattern:
		// Handle nested object parameter patterns (from function parameters)
		c.checkNestedObjectParameterPattern(targetNode, expectedType, context)
	case *parser.MemberExpression:
		// Member access as target: [obj.prop] = [value]
		// Type check the member expression and ensure it's assignable
		c.visit(targetNode)
	case *parser.IndexExpression:
		// Index access as target: [arr[0]] = [value]
		// Type check the index expression and ensure it's assignable
		c.visit(targetNode)
	case *parser.AssignmentExpression, *parser.SpreadElement:
		// Nested default or rest - see unwrapNestedPatternTarget.
		inner, innerType := c.unwrapNestedPatternTarget(target, expectedType)
		c.checkDestructuringTarget(inner, innerType, context)
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
	case *parser.ArrayParameterPattern:
		c.checkNestedArrayParameterPattern(targetNode, expectedType, propName)
	case *parser.ObjectParameterPattern:
		c.checkNestedObjectParameterPattern(targetNode, expectedType, propName)
	case *parser.MemberExpression:
		// Member access as target: {prop: obj.field} = {prop: value}
		c.visit(targetNode)
	case *parser.IndexExpression:
		// Index access as target: {prop: arr[0]} = {prop: value}
		c.visit(targetNode)
	case *parser.AssignmentExpression, *parser.SpreadElement:
		// Nested default or rest - see unwrapNestedPatternTarget.
		inner, innerType := c.unwrapNestedPatternTarget(target, expectedType)
		c.checkDestructuringTargetForProperty(inner, innerType, propName)
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
				if spread, isRest := prop.Key.(*parser.SpreadElement); isRest {
					// A rest element nested inside an object pattern
					// (`{a: {b, ...rr}}`) - see nestedObjectPatternRestType.
					c.checkDestructuringTargetForProperty(spread.Argument, nestedObjectPatternRestType(expectedType, objectTarget), "")
					continue
				}
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
			if spread, isRest := prop.Key.(*parser.SpreadElement); isRest {
				c.checkDestructuringTargetForProperty(spread.Argument, types.Any, "")
				continue
			}
			c.checkDestructuringTargetForProperty(prop.Value, types.Any, "")
		}
	}
}

// declarationEnv returns the scope a declaration's bindings belong in. `var` is
// function-scoped: its names go in the nearest function (or global) scope no
// matter how many blocks deep the declaration sits, which is what the plain
// VarStatement path already does via GetFunctionScope. let/const are
// block-scoped and belong in the current scope.
//
// Defining a `var` pattern's bindings in the current scope instead is what made
// them invisible after the enclosing block: `function f(){ { var {w} = o; }
// return w; }` reported TS2304 even though the runtime binds w at function
// scope.
func (c *Checker) declarationEnv(isVar bool) *Environment {
	if isVar {
		return c.env.GetFunctionScope()
	}
	return c.env
}

// checkDestructuringTargetForDeclaration handles type checking and environment definition for destructuring targets in declarations
func (c *Checker) checkDestructuringTargetForDeclaration(target parser.Expression, expectedType types.Type, isConst bool, isVar bool) {
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

		env := c.declarationEnv(isVar)
		if !env.Define(targetNode.Value, finalType, isConst) {
			if isVar {
				// var redeclaration is legal; keep the plain-VarStatement path's
				// behavior of updating the existing binding.
				env.Update(targetNode.Value, finalType)
			} else {
				c.addError(targetNode, fmt.Sprintf("identifier '%s' already declared", targetNode.Value))
			}
		}
		targetNode.SetComputedType(finalType)
	case *parser.ArrayLiteral:
		// Nested array destructuring declaration
		c.checkNestedArrayTargetForDeclaration(targetNode, expectedType, isConst, isVar)
	case *parser.ObjectLiteral:
		// Nested object destructuring declaration
		c.checkNestedObjectTargetForDeclaration(targetNode, expectedType, isConst, isVar)
	case *parser.ArrayParameterPattern:
		// Parameter patterns in declarations (shouldn't happen normally, but handle for consistency)
		c.checkNestedArrayParameterPatternForDeclaration(targetNode, expectedType, isConst, isVar)
	case *parser.ObjectParameterPattern:
		// Parameter patterns in declarations (shouldn't happen normally, but handle for consistency)
		c.checkNestedObjectParameterPatternForDeclaration(targetNode, expectedType, isConst, isVar)
	case *parser.UndefinedLiteral:
		// Elision in destructuring - no type checking needed, just skip this element
		return
	case *parser.MemberExpression:
		// Member access as target: const [obj.prop] = [value]
		// This is valid in JavaScript (though less common in declarations)
		c.visit(targetNode)
	case *parser.IndexExpression:
		// Index access as target: const [arr[0]] = [value]
		// This is valid in JavaScript (though less common in declarations)
		c.visit(targetNode)
	case *parser.AssignmentExpression, *parser.SpreadElement:
		// Nested default or rest - see unwrapNestedPatternTarget.
		inner, innerType := c.unwrapNestedPatternTarget(target, expectedType)
		c.checkDestructuringTargetForDeclaration(inner, innerType, isConst, isVar)
	default:
		c.addError(target, fmt.Sprintf("invalid destructuring target type: %T", target))
	}
}

// checkNestedArrayTargetForDeclaration handles type checking for nested array destructuring in declarations
func (c *Checker) checkNestedArrayTargetForDeclaration(arrayTarget *parser.ArrayLiteral, expectedType types.Type, isConst bool, isVar bool) {
	// Validate that expectedType is array-like
	widenedType := types.GetWidenedType(expectedType)
	var elementType types.Type

	if arrayType, ok := widenedType.(*types.ArrayType); ok {
		elementType = arrayType.ElementType
	} else if tupleType, ok := widenedType.(*types.TupleType); ok {
		// For tuple types, check each element with its specific type
		for i, element := range arrayTarget.Elements {
			// A rest element consumes the remaining tuple members, so it gets an
			// array of their union - not element i's type, which would
			// over-narrow and silently accept e.g. string[] for a
			// (string|boolean)[] value.
			if spread, ok := element.(*parser.SpreadElement); ok {
				c.checkDestructuringTargetForDeclaration(spread.Argument, restElementType(tupleType, i), isConst, isVar)
				continue
			}
			var elemType types.Type
			if i < len(tupleType.ElementTypes) {
				elemType = tupleType.ElementTypes[i]
			} else {
				elemType = types.Undefined
			}
			c.checkDestructuringTargetForDeclaration(element, elemType, isConst, isVar)
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
			c.checkNestedArrayTargetForDeclaration(arrayTarget, arrayLikeType, isConst, isVar)
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
		c.checkDestructuringTargetForDeclaration(element, elementType, isConst, isVar)
	}
}

// checkNestedObjectTargetForDeclaration handles type checking for nested object destructuring in declarations
func (c *Checker) checkNestedObjectTargetForDeclaration(objectTarget *parser.ObjectLiteral, expectedType types.Type, isConst bool, isVar bool) {
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
				if spread, isRest := prop.Key.(*parser.SpreadElement); isRest {
					// A rest element nested inside an object pattern
					// (`{a: {b, ...rr}}`): it gets the source's remaining
					// properties, like a top-level object rest.
					c.checkDestructuringTargetForDeclaration(spread.Argument, nestedObjectPatternRestType(expectedType, objectTarget), isConst, isVar)
					continue
				}
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

			c.checkDestructuringTargetForDeclaration(prop.Value, propType, isConst, isVar)
		}
	} else {
		// For Any type, all nested targets get Any type
		for _, prop := range objectTarget.Properties {
			if spread, isRest := prop.Key.(*parser.SpreadElement); isRest {
				c.checkDestructuringTargetForDeclaration(spread.Argument, types.Any, isConst, isVar)
				continue
			}
			c.checkDestructuringTargetForDeclaration(prop.Value, types.Any, isConst, isVar)
		}
	}
}

// checkNestedArrayParameterPattern handles type checking for nested array parameter patterns
func (c *Checker) checkNestedArrayParameterPattern(pattern *parser.ArrayParameterPattern, expectedType types.Type, context interface{}) {
	// ArrayParameterPattern has Elements field with DestructuringElement items
	// Process each element similar to array literals
	widenedType := types.GetWidenedType(expectedType)
	var elementType types.Type

	if arrayType, ok := widenedType.(*types.ArrayType); ok {
		elementType = arrayType.ElementType
	} else if widenedType == types.Any {
		elementType = types.Any
	} else {
		c.addError(pattern, fmt.Sprintf("cannot destructure array pattern from non-array type '%s'", expectedType.String()))
		elementType = types.Any
	}

	for _, elem := range pattern.Elements {
		if elem == nil || elem.Target == nil {
			// Elision
			continue
		}
		// Recursively check the target
		c.checkDestructuringTarget(elem.Target, elementType, context)
	}
}

// checkNestedObjectParameterPattern handles type checking for nested object parameter patterns
func (c *Checker) checkNestedObjectParameterPattern(pattern *parser.ObjectParameterPattern, expectedType types.Type, context interface{}) {
	// ObjectParameterPattern has Properties field with DestructuringProperty items
	widenedType := types.GetWidenedType(expectedType)

	if widenedType != types.Any {
		objType, ok := expectedType.(*types.ObjectType)
		if !ok {
			c.addError(pattern, fmt.Sprintf("cannot destructure object pattern from non-object type '%s'", expectedType.String()))
			return
		}

		for _, prop := range pattern.Properties {
			if prop == nil {
				continue
			}

			var propName string
			if ident, ok := prop.Key.(*parser.Identifier); ok {
				propName = ident.Value
			}

			var propType types.Type = types.Undefined
			if t, exists := objType.Properties[propName]; exists {
				propType = t
			}

			// Recursively check the target
			if prop.Target != nil {
				c.checkDestructuringTarget(prop.Target, propType, propName)
			} else {
				c.checkDestructuringTarget(prop.Key, propType, propName)
			}
		}
	} else {
		// For Any type, all nested targets get Any type
		for _, prop := range pattern.Properties {
			if prop == nil {
				continue
			}
			if prop.Target != nil {
				c.checkDestructuringTarget(prop.Target, types.Any, nil)
			} else {
				c.checkDestructuringTarget(prop.Key, types.Any, nil)
			}
		}
	}
}

// checkNestedArrayParameterPatternForDeclaration handles type checking for nested array parameter patterns in declarations
func (c *Checker) checkNestedArrayParameterPatternForDeclaration(pattern *parser.ArrayParameterPattern, expectedType types.Type, isConst bool, isVar bool) {
	widenedType := types.GetWidenedType(expectedType)
	var elementType types.Type

	if arrayType, ok := widenedType.(*types.ArrayType); ok {
		elementType = arrayType.ElementType
	} else {
		elementType = types.Any
	}

	for _, elem := range pattern.Elements {
		if elem == nil || elem.Target == nil {
			continue
		}
		c.checkDestructuringTargetForDeclaration(elem.Target, elementType, isConst, isVar)
	}
}

// checkNestedObjectParameterPatternForDeclaration handles type checking for nested object parameter patterns in declarations
func (c *Checker) checkNestedObjectParameterPatternForDeclaration(pattern *parser.ObjectParameterPattern, expectedType types.Type, isConst bool, isVar bool) {
	widenedType := types.GetWidenedType(expectedType)

	if widenedType != types.Any {
		objType, ok := expectedType.(*types.ObjectType)
		if !ok {
			// For non-object types, use Any for all properties
			for _, prop := range pattern.Properties {
				if prop == nil {
					continue
				}
				if prop.Target != nil {
					c.checkDestructuringTargetForDeclaration(prop.Target, types.Any, isConst, isVar)
				} else {
					c.checkDestructuringTargetForDeclaration(prop.Key, types.Any, isConst, isVar)
				}
			}
			return
		}

		for _, prop := range pattern.Properties {
			if prop == nil {
				continue
			}

			var propName string
			if ident, ok := prop.Key.(*parser.Identifier); ok {
				propName = ident.Value
			}

			var propType types.Type = types.Undefined
			if t, exists := objType.Properties[propName]; exists {
				propType = t
			}

			if prop.Target != nil {
				c.checkDestructuringTargetForDeclaration(prop.Target, propType, isConst, isVar)
			} else {
				c.checkDestructuringTargetForDeclaration(prop.Key, propType, isConst, isVar)
			}
		}
	} else {
		// For Any type, all nested targets get Any type
		for _, prop := range pattern.Properties {
			if prop == nil {
				continue
			}
			if prop.Target != nil {
				c.checkDestructuringTargetForDeclaration(prop.Target, types.Any, isConst, isVar)
			} else {
				c.checkDestructuringTargetForDeclaration(prop.Key, types.Any, isConst, isVar)
			}
		}
	}
}

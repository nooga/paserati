package checker

import (
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// isFreshLiteralExpression reports whether node is literal syntax written
// directly in source (`"foo"`, `42`, `true`, `1n`, or a unary-minus number
// literal). Only a type computed straight from one of these is a "fresh"
// literal type that a `let`/`var` declaration without a type annotation
// should widen to its base primitive.
//
// A literal type can also arise as the *result* of an expression — e.g.
// `a || "foo"` collapsing to the literal type "foo" via subtype reduction —
// without the expression being literal syntax itself. TypeScript does not
// widen those: freshness is a property of the literal token, not of
// whatever type happens to describe the expression's value.
func isFreshLiteralExpression(node parser.Node) bool {
	switch n := node.(type) {
	case *parser.StringLiteral, *parser.NumberLiteral, *parser.BooleanLiteral, *parser.BigIntLiteral:
		return true
	case *parser.PrefixExpression:
		if n.Operator == "-" || n.Operator == "+" {
			return isFreshLiteralExpression(n.Right)
		}
	}
	return false
}

// Initialize the prototype method resolver
func init() {
	// Set the prototype method resolver to use our new environment-based approach
	types.SetPrototypeMethodResolver(getPrototypeMethodTypeFromGlobalEnv)
}

// getPrototypeMethodTypeFromGlobalEnv is the new prototype method resolver
// that uses the environment's primitive prototype registry
func getPrototypeMethodTypeFromGlobalEnv(primitiveName, methodName string) types.Type {
	if globalEnvironment != nil {
		return globalEnvironment.GetPrimitivePrototypeMethodType(primitiveName, methodName)
	}
	return nil
}

// getBuiltinType looks up a builtin type by name in the global environment
func (c *Checker) getBuiltinType(name string) types.Type {
	if typ, _, found := c.env.Resolve(name); found {
		return typ
	}
	return nil
}

// getPropertyTypeFromType returns the type of a property access on the given type
// isOptionalChaining determines whether to be permissive about missing properties
// This is now a wrapper around the implementation in the types package.
func (c *Checker) getPropertyTypeFromType(objectType types.Type, propertyName string, isOptionalChaining bool) types.Type {
	return types.GetPropertyType(objectType, propertyName, isOptionalChaining)
}

// resolveObjectMemberForDestructuring resolves a property access on an object
// type for destructuring. It mirrors member-access resolution (see
// checkMemberExpression): inherited properties, then index signatures, then the
// object/function prototype. Returns the resolved property type and whether the
// property exists. Destructuring must use this rather than a bare Properties
// lookup, otherwise inherited or index-signature properties are wrongly reported
// as missing.
func (c *Checker) resolveObjectMemberForDestructuring(obj *types.ObjectType, propName string) (types.Type, bool) {
	if obj == nil {
		return types.Undefined, false
	}

	// Return the bare property type (no `| undefined` for optional members):
	// the destructuring callers fold in defaults and undefined themselves, and
	// widening optional props here defeats their default-collapsing logic.
	if propType, exists := obj.GetEffectiveProperties()[propName]; exists {
		if propType == nil {
			return types.Undefined, false
		}
		return propType, true
	}

	for _, indexSig := range obj.IndexSignatures {
		if indexSig.KeyType == types.String || indexSig.KeyType == types.Any {
			if indexSig.ValueType == nil {
				return types.Any, true
			}
			return indexSig.ValueType, true
		}
	}

	if obj.IsCallable() {
		if methodType := c.env.GetPrimitivePrototypeMethodType("function", propName); methodType != nil {
			return methodType, true
		}
	}
	if methodType := c.env.GetPrimitivePrototypeMethodType("object", propName); methodType != nil {
		return methodType, true
	}

	return types.Undefined, false
}

// objectLikeUnionMembers returns the ObjectType constituents of a union when
// every member is itself object-like, or ok=false if any member isn't (e.g.
// a primitive or array), in which case the union can't be treated as a
// single destructurable/spreadable shape.
func objectLikeUnionMembers(union *types.UnionType) (members []*types.ObjectType, ok bool) {
	for _, member := range union.Types {
		if nested, isUnion := member.(*types.UnionType); isUnion {
			nestedMembers, nestedOk := objectLikeUnionMembers(nested)
			if !nestedOk {
				return nil, false
			}
			members = append(members, nestedMembers...)
			continue
		}
		obj, isObj := member.(*types.ObjectType)
		if !isObj {
			return nil, false
		}
		members = append(members, obj)
	}
	return members, true
}

// resolveObjectMemberForDestructuringUnion resolves a property across the
// object-like constituents of a union type. A JavaScript destructuring
// pattern never fails at runtime just because a key is absent — it simply
// yields `undefined` for that key — so unlike a direct property access on a
// union (which TypeScript requires to exist on every member), a property
// found on at least one constituent resolves here, with `undefined` folded
// in whenever some constituent lacks it.
func (c *Checker) resolveObjectMemberForDestructuringUnion(members []*types.ObjectType, propName string) (types.Type, bool) {
	var found []types.Type
	missingOnSome := false
	for _, obj := range members {
		if pt, exists := c.resolveObjectMemberForDestructuring(obj, propName); exists {
			found = append(found, pt)
		} else {
			missingOnSome = true
		}
	}
	if len(found) == 0 {
		return types.Undefined, false
	}
	if missingOnSome {
		found = append(found, types.Undefined)
	}
	return types.NewUnionType(found...), true
}

// validateIndexSignatures checks if a source object type satisfies the index signature constraints of a target type
// This is used when assigning object literals to types with index signatures
func (c *Checker) validateIndexSignatures(sourceType, targetType types.Type) []IndexSignatureError {
	var errors []IndexSignatureError

	sourceObj, sourceIsObj := sourceType.(*types.ObjectType)
	targetObj, targetIsObj := targetType.(*types.ObjectType)

	// Only validate if both are object types and target has index signatures
	if !sourceIsObj || !targetIsObj || len(targetObj.IndexSignatures) == 0 {
		return errors
	}

	// Check each property in source against all index signatures in target
	for propName, propType := range sourceObj.Properties {
		errors = append(errors, c.validatePropertyAgainstIndexSignatures(propName, propType, targetObj.IndexSignatures)...)
	}

	return errors
}

// validatePropertyAgainstIndexSignatures checks if a single property satisfies index signature constraints
func (c *Checker) validatePropertyAgainstIndexSignatures(propName string, propType types.Type, indexSignatures []*types.IndexSignature) []IndexSignatureError {
	var errors []IndexSignatureError

	for _, indexSig := range indexSignatures {
		if c.propertyMatchesIndexSignature(propName, indexSig) {
			// Property matches this index signature's key pattern, validate value type
			if !types.IsAssignable(propType, indexSig.ValueType) {
				errors = append(errors, IndexSignatureError{
					PropertyName: propName,
					PropertyType: propType,
					ExpectedType: indexSig.ValueType,
					KeyType:      indexSig.KeyType,
				})
			}
		}
	}

	return errors
}

// propertyMatchesIndexSignature determines if a property name matches an index signature's key type
func (c *Checker) propertyMatchesIndexSignature(propName string, indexSig *types.IndexSignature) bool {
	// For now, we only support string and number key types
	switch indexSig.KeyType {
	case types.String:
		// All string property names match string index signatures
		return true
	case types.Number:
		// Only numeric property names match number index signatures
		// In JavaScript, array indices and numeric properties are treated as numbers
		// For simplicity, we'll check if the property name looks numeric
		for _, char := range propName {
			if char < '0' || char > '9' {
				return false
			}
		}
		return len(propName) > 0
	default:
		// For other key types (like union types), we'd need more sophisticated matching
		return false
	}
}

// IndexSignatureError represents an error when a property doesn't match index signature constraints
type IndexSignatureError struct {
	PropertyName string
	PropertyType types.Type
	ExpectedType types.Type
	KeyType      types.Type
}

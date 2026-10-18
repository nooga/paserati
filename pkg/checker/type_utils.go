package checker

import (
	"strings"

	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
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
	case *parser.TemplateLiteral:
		_, ok := noSubstitutionTemplateText(n)
		return ok
	case *parser.PrefixExpression:
		if n.Operator == "-" || n.Operator == "+" {
			return isFreshLiteralExpression(n.Right)
		}
	}
	return false
}

// constLiteralType is the type of an unannotated const whose initializer has
// literal type computed: a fresh literal becomes a widening literal type.
func (c *Checker) constLiteralType(initializer parser.Node, computed types.Type) types.Type {
	lt, ok := computed.(*types.LiteralType)
	if !ok || !isFreshLiteralExpression(initializer) {
		return computed
	}
	w := &types.LiteralType{Value: lt.Value}
	if c.wideningLiterals == nil {
		c.wideningLiterals = map[*types.LiteralType]bool{}
	}
	c.wideningLiterals[w] = true
	return w
}

// isWideningLiteral reports whether t is a const's widening literal type.
func (c *Checker) isWideningLiteral(t types.Type) bool {
	lt, ok := t.(*types.LiteralType)
	return ok && c.wideningLiterals[lt]
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
	return c.rel.GetPropertyType(objectType, propertyName, isOptionalChaining)
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
	if propType, exists := obj.EffectiveProperty(propName); exists {
		if propType == nil {
			return types.Undefined, false
		}
		return propType, true
	}

	for _, indexSig := range obj.IndexSigs() {
		if indexSig.KeyType == types.String || indexSig.KeyType == types.Any {
			if indexSig.ValueType == nil {
				return types.Any, true
			}
			return indexSig.ValueType, true
		}
	}

	if (obj.IsCallable() || obj.IsConstructable()) {
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

// destructurableObject is the object an object pattern reads properties from
// when t is an intersection or a mapped type (`MergeC<A, B> = A & B`), and t
// otherwise.
func (c *Checker) destructurableObject(t types.Type) types.Type {
	if eff := c.resolveStructural(types.GetEffectiveType(t)); eff != nil {
		if _, ok := eff.(*types.IntersectionType); ok {
			t = eff
		}
	}
	switch t.(type) {
	case *types.IntersectionType, *types.MappedType:
		if obj, ok := c.structuralObject(t); ok {
			return obj
		}
	}
	return t
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
	if !sourceIsObj || !targetIsObj || len(targetObj.IndexSigs()) == 0 {
		return errors
	}

	// Check each property in source against all index signatures in target
	for propName, propType := range sourceObj.Props() {
		errors = append(errors, c.validatePropertyAgainstIndexSignatures(propName, propType, targetObj.IndexSigs())...)
	}

	return errors
}

// validatePropertyAgainstIndexSignatures checks if a single property satisfies index signature constraints
func (c *Checker) validatePropertyAgainstIndexSignatures(propName string, propType types.Type, indexSignatures []*types.IndexSignature) []IndexSignatureError {
	var errors []IndexSignatureError

	for _, indexSig := range indexSignatures {
		if c.propertyMatchesIndexSignature(propName, indexSig) {
			// Property matches this index signature's key pattern, validate value type
			if !c.rel.IsAssignable(propType, indexSig.ValueType) {
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

// noSubstitutionTemplateText returns the text of a template literal made only
// of string parts (no `${}` substitutions).
func noSubstitutionTemplateText(node *parser.TemplateLiteral) (string, bool) {
	if node == nil {
		return "", false
	}
	var sb strings.Builder
	for _, part := range node.Parts {
		sp, ok := part.(*parser.TemplateStringPart)
		if !ok || sp == nil {
			return "", false
		}
		sb.WriteString(sp.Value)
	}
	return sb.String(), true
}

// readonlyArrayMutators are the Array.prototype methods ReadonlyArray<T>
// leaves out.
var readonlyArrayMutators = map[string]bool{
	"push": true, "pop": true, "shift": true, "unshift": true, "splice": true,
	"sort": true, "reverse": true, "fill": true, "copyWithin": true,
}

// apparentType is the type whose members a read of t sees: a mapped type
// (Record<string, T>, Readonly<T>, ...) as its expanded object form, and an
// intersection with each member so expanded (#634).
func (c *Checker) apparentType(t types.Type) types.Type {
	switch tt := t.(type) {
	case *types.MappedType, *types.InstantiatedType:
		if e := c.expandIfMappedType(t); e != nil {
			return e
		}
	case *types.IntersectionType:
		changed := false
		members := make([]types.Type, len(tt.Types))
		for i, m := range tt.Types {
			members[i] = c.apparentType(m)
			changed = changed || members[i] != m
		}
		if changed {
			return types.NewIntersectionType(members...)
		}
	case *types.UnionType:
		changed := false
		members := make([]types.Type, len(tt.Types))
		for i, m := range tt.Types {
			members[i] = c.apparentType(m)
			changed = changed || members[i] != m
		}
		if changed {
			return types.NewUnionType(members...)
		}
	}
	return t
}

// stripReadonlyArrays turns `readonly T[]` / `readonly [A, B]`, alone or as
// union members, into their mutable forms, for positions that only read the
// elements (iteration).
func stripReadonlyArrays(t types.Type) types.Type {
	switch tt := t.(type) {
	case *types.ReadonlyType:
		switch tt.InnerType.(type) {
		case *types.ArrayType, *types.TupleType:
			return tt.InnerType
		}
	case *types.UnionType:
		changed := false
		members := make([]types.Type, len(tt.Types))
		for i, m := range tt.Types {
			members[i] = stripReadonlyArrays(m)
			changed = changed || members[i] != m
		}
		if changed {
			return types.NewUnionType(members...)
		}
	}
	return t
}

// isPrimitiveDestructuringSource reports whether t is a string, number,
// boolean or bigint type (or a literal of one), destructured through its
// apparent type.
func isPrimitiveDestructuringSource(t types.Type) bool {
	switch t {
	case types.String, types.Number, types.Boolean, types.BigInt:
		return true
	}
	if lt, ok := t.(*types.LiteralType); ok {
		switch lt.Value.Type() {
		case vm.TypeString, vm.TypeFloatNumber, vm.TypeIntegerNumber, vm.TypeBoolean, vm.TypeBigInt:
			return true
		}
	}
	return false
}

// primitiveApparentName is the name of a primitive's apparent type, as tsc
// prints it in member errors.
func primitiveApparentName(t types.Type) string {
	base := t
	if lt, ok := t.(*types.LiteralType); ok {
		switch lt.Value.Type() {
		case vm.TypeString:
			base = types.String
		case vm.TypeBoolean:
			base = types.Boolean
		case vm.TypeBigInt:
			base = types.BigInt
		default:
			base = types.Number
		}
	}
	switch base {
	case types.String:
		return "String"
	case types.Boolean:
		return "Boolean"
	case types.BigInt:
		return "BigInt"
	}
	return "Number"
}

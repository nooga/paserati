package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// primitiveBaseTypeNames are the keyword names TypeScript resolves as values in
// a heritage clause and rejects with TS2840 (isPrimitiveTypeName).
var primitiveBaseTypeNames = map[string]bool{
	"any": true, "string": true, "number": true, "boolean": true, "never": true, "unknown": true,
}

// interfaceBaseObjects returns the object types whose members an interface
// inherits from a base type that is not a plain object type, and whether the
// base is a valid interface base at all (TypeScript's isValidBaseType): arrays,
// tuples, `any`, and intersections of valid bases are.
func (c *Checker) interfaceBaseObjects(t types.Type) ([]*types.ObjectType, bool) {
	t = types.GetEffectiveType(t)
	switch bt := t.(type) {
	case *types.ObjectType:
		return []*types.ObjectType{bt}, true
	case *types.ArrayType:
		return []*types.ObjectType{c.arrayMembersObject(bt.ElementType, false)}, true
	case *types.TupleType:
		return nil, true
	case *types.IntersectionType:
		var objs []*types.ObjectType
		for _, member := range bt.Types {
			sub, ok := c.interfaceBaseObjects(member)
			if !ok {
				return nil, false
			}
			objs = append(objs, sub...)
		}
		return objs, true
	case *types.ReadonlyType:
		if arr, ok := bt.InnerType.(*types.ArrayType); ok {
			return []*types.ObjectType{c.arrayMembersObject(arr.ElementType, true)}, true
		}
		return c.interfaceBaseObjects(bt.InnerType)
	case *types.InstantiatedType:
		if sub := types.GetEffectiveType(bt.Substitute()); sub != t {
			return c.interfaceBaseObjects(sub)
		}
	}
	if t == types.Any {
		return nil, true
	}
	return nil, false
}

// reportInvalidInterfaceBase reports an interface whose extends-clause entry is
// not a valid base type: TS2840 for the primitive keyword names, TS2312 for
// anything else that is not an object type.
func (c *Checker) reportInvalidInterfaceBase(expr parser.Expression, baseType types.Type) {
	if id, ok := expr.(*parser.Identifier); ok && primitiveBaseTypeNames[id.Value] {
		c.addErrorWithCode(expr, errors.TS2840, fmt.Sprintf("An interface cannot extend a primitive type like '%s'. It can only extend other named object types.", id.Value))
		return
	}
	c.addErrorWithCode(expr, errors.TS2312, "An interface can only extend an object type or intersection of object types with statically known members.")
}

// mutatingArrayMethods are the Array members ReadonlyArray lacks.
var mutatingArrayMethods = map[string]bool{
	"push": true, "pop": true, "shift": true, "unshift": true, "splice": true,
	"sort": true, "reverse": true, "fill": true, "copyWithin": true,
}

// arrayMembersObject is the object an interface extending an array type
// inherits: `length` and the Array methods for the element type.
func (c *Checker) arrayMembersObject(elem types.Type, readonly bool) *types.ObjectType {
	obj := types.NewObjectType()
	obj.SetProperty("length", types.Number)
	if proto := c.env.PrimitivePrototypeType("array"); proto != nil {
		for _, name := range proto.PropertyNames() {
			if readonly && mutatingArrayMethods[name] {
				continue
			}
			obj.SetProperty(name, c.instantiateGenericMethod(proto.Props()[name], elem))
		}
	}
	return obj
}

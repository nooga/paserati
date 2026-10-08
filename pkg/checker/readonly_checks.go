package checker

import (
	"fmt"
	"strconv"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// setReadonlyProperty records (or clears) a property's readonly flag.
func setReadonlyProperty(obj *types.ObjectType, name string, readonly bool) {
	if obj.ReadOnlyProperties == nil {
		if !readonly {
			return
		}
		obj.ReadOnlyProperties = map[string]bool{}
	}
	if readonly {
		obj.ReadOnlyProperties[name] = true
	} else {
		delete(obj.ReadOnlyProperties, name)
	}
}

// checkReadonlyAssignmentTarget reports a write (=, op=, ++, --) to a
// read-only property or index: TS2540 for a named property, TS2542 for a
// readonly index signature or array. It returns whether it reported.
func (c *Checker) checkReadonlyAssignmentTarget(target parser.Expression) bool {
	var objectExpr parser.Expression
	var name string
	named := false
	switch t := target.(type) {
	case *parser.MemberExpression:
		objectExpr = t.Object
		if t.Property == nil {
			return false
		}
		name, named = c.extractPropertyName(t.Property), true
	case *parser.IndexExpression:
		objectExpr = t.Left
		switch idx := t.Index.(type) {
		case *parser.StringLiteral:
			name, named = idx.Value, true
		case *parser.NumberLiteral:
			name, named = strconv.FormatFloat(idx.Value, 'f', -1, 64), true
		}
	default:
		return false
	}
	objectType := objectExpr.GetComputedType()
	if objectType == nil {
		return false
	}

	if named {
		if !c.isReadonlyProperty(objectType, name, 0) {
			if !c.hasProperty(objectType, name) && c.hasReadonlyIndex(objectType, name) {
				c.addErrorWithCode(target, errors.TS2542, fmt.Sprintf("Index signature in type '%s' only permits reading.", objectType.String()))
				return true
			}
			return false
		}
		// A class's readonly fields are assignable in its own constructor.
		if c.currentClassContext != nil &&
			c.currentClassContext.ContextType == types.AccessContextConstructor &&
			c.isThisExpression(objectExpr) {
			return false
		}
		c.addErrorWithCode(target, errors.TS2540, fmt.Sprintf("Cannot assign to '%s' because it is a read-only property.", name))
		return true
	}
	if c.hasReadonlyIndex(objectType, "") {
		c.addErrorWithCode(target, errors.TS2542, fmt.Sprintf("Index signature in type '%s' only permits reading.", objectType.String()))
		return true
	}
	return false
}

// isReadonlyProperty reports whether writing property name of t is
// rejected: readonly members of object types, interfaces and classes
// (inherited too), Readonly<T> and other mapped types, `as const` objects,
// and the elements and length of readonly tuples and arrays.
func (c *Checker) isReadonlyProperty(t types.Type, name string, depth int) bool {
	if t == nil || depth > 10 {
		return false
	}
	t = c.apparentType(types.GetWidenedType(t))
	switch tt := t.(type) {
	case *types.ReadonlyType:
		switch inner := tt.InnerType.(type) {
		case *types.TupleType:
			if name == "length" {
				return true
			}
			i, err := strconv.Atoi(name)
			return err == nil && i >= 0 && (i < len(inner.ElementTypes) || inner.RestElementType != nil)
		case *types.ArrayType:
			return name == "length"
		case *types.ObjectType:
			if _, ok := inner.GetEffectiveProperties()[name]; ok {
				return true
			}
		}
		return c.isReadonlyProperty(tt.InnerType, name, depth+1)
	case *types.ObjectType:
		return c.objectPropertyReadonly(tt, name, depth)
	case *types.UnionType:
		for _, m := range tt.Types {
			if c.isReadonlyProperty(m, name, depth+1) {
				return true
			}
		}
	case *types.IntersectionType:
		// Writable if any constituent declaring it declares it writable.
		declared, readonly := 0, 0
		for _, m := range tt.Types {
			if c.hasProperty(m, name) {
				declared++
				if c.isReadonlyProperty(m, name, depth+1) {
					readonly++
				}
			}
		}
		return declared > 0 && readonly == declared
	}
	return false
}

func (c *Checker) objectPropertyReadonly(obj *types.ObjectType, name string, depth int) bool {
	if propType, own := obj.Properties[name]; own {
		if obj.ReadOnlyProperties[name] || types.IsReadonlyType(propType) {
			return true
		}
		if obj.ClassMeta != nil {
			if info := obj.ClassMeta.GetMemberAccess(name); info != nil && info.IsReadonly {
				return true
			}
		}
		return false
	}
	for _, base := range obj.BaseTypes {
		if c.hasProperty(base, name) {
			return c.isReadonlyProperty(base, name, depth+1)
		}
	}
	return false
}

// hasProperty reports whether t declares property name (own or inherited).
func (c *Checker) hasProperty(t types.Type, name string) bool {
	switch tt := c.apparentType(types.GetWidenedType(t)).(type) {
	case *types.ObjectType:
		_, ok := tt.GetEffectiveProperties()[name]
		return ok
	case *types.ReadonlyType:
		return c.hasProperty(tt.InnerType, name)
	}
	return false
}

// hasReadonlyIndex reports whether writes through t's index signature are
// rejected: readonly arrays and `readonly [k: K]: V` signatures. A non-empty
// name restricts it to the signatures that name could match.
func (c *Checker) hasReadonlyIndex(t types.Type, name string) bool {
	switch tt := c.apparentType(types.GetWidenedType(t)).(type) {
	case *types.ReadonlyType:
		switch tt.InnerType.(type) {
		case *types.ArrayType:
			return true
		case *types.TupleType:
			return name == ""
		}
		return c.hasReadonlyIndex(tt.InnerType, name)
	case *types.ObjectType:
		for _, sig := range tt.IndexSignatures {
			if !sig.Readonly {
				continue
			}
			if name == "" || sig.KeyType == types.String || sig.KeyType == types.Any {
				return true
			}
			if sig.KeyType == types.Number {
				if _, err := strconv.ParseFloat(name, 64); err == nil {
					return true
				}
			}
		}
	}
	return false
}

// hasMutableArrayLikeMember reports whether t, or a member of union t, is a
// mutable array or tuple type.
func hasMutableArrayLikeMember(t types.Type) bool {
	switch tt := t.(type) {
	case *types.ArrayType, *types.TupleType:
		return true
	case *types.UnionType:
		for _, m := range tt.Types {
			if hasMutableArrayLikeMember(m) {
				return true
			}
		}
	}
	return false
}

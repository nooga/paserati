package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// iterableMarkerProperties are members whose presence on an object type means
// we cannot be sure it lacks a [Symbol.iterator]() method: computed symbol keys
// are stored under a placeholder name, and the collection types expose these.
var iterableMarkerProperties = []string{"__COMPUTED_PROPERTY__", "entries", "values", "keys", "[Symbol.iterator]"}

// isDefinitelyNotIterable reports whether a type certainly has no
// [Symbol.iterator]() method. Types we cannot judge (type parameters, deferred
// types, collections, anything with an iterator-looking member) answer false so
// that no diagnostic is raised for them.
func (c *Checker) isDefinitelyNotIterable(t types.Type) bool {
	if t == nil {
		return false
	}
	t = types.GetEffectiveType(t)
	switch tt := t.(type) {
	case *types.Primitive:
		switch tt {
		case types.Number, types.Boolean, types.Symbol, types.BigInt, types.Void, types.Null, types.Undefined:
			return true
		}
		return false
	case *types.LiteralType:
		return !types.IsAssignable(tt, types.String)
	case *types.ObjectType:
		if tt.IsCallable() || len(tt.ConstructSignatures) > 0 || len(tt.BaseTypes) > 0 || tt.ClassMeta != nil {
			return false
		}
		for _, marker := range iterableMarkerProperties {
			if _, ok := tt.Properties[marker]; ok {
				return false
			}
		}
		return true
	case *types.UnionType:
		for _, member := range tt.Types {
			if member == types.Null || member == types.Undefined {
				return false
			}
		}
		for _, member := range tt.Types {
			if c.isDefinitelyNotIterable(member) {
				return true
			}
		}
	}
	return false
}

// reportNotIterable reports TS2488 when t certainly cannot be iterated.
func (c *Checker) reportNotIterable(node parser.Node, t types.Type) {
	if !c.isDefinitelyNotIterable(t) {
		return
	}
	c.addErrorWithCode(node, errors.TS2488, fmt.Sprintf("Type '%s' must have a '[Symbol.iterator]()' method that returns an iterator.", t.String()))
}

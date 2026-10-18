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
		return !c.rel.IsAssignable(tt, types.String)
	case *types.ObjectType:
		if len(tt.IndexSigs()) > 0 || len(tt.ConstructSigs()) > 0 {
			return false
		}
		for _, marker := range iterableMarkerProperties {
			if pt, ok := tt.EffectiveProperty(marker); ok {
				return c.iteratorMethodIsBroken(tt, marker, pt)
			}
		}
		// Only judge what we fully see: a plain function, a class instance,
		// or an object literal type whose bases are all known.
		for _, base := range tt.Bases() {
			if _, ok := base.(*types.ObjectType); !ok {
				return false
			}
		}
		return true
	case *types.UnionType:
		for _, member := range tt.Types {
			if member == types.Null || member == types.Undefined {
				return c.strictNullChecks
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

// iteratorMethodIsBroken judges a [Symbol.iterator] member we can see: an
// optional one (under strictNullChecks), or a method returning an object
// that has no next() method, does not make the type iterable. Anything less
// certain counts as iterable.
func (c *Checker) iteratorMethodIsBroken(owner *types.ObjectType, marker string, member types.Type) bool {
	isIteratorKey := marker == "[Symbol.iterator]" || (marker == "__COMPUTED_PROPERTY__" && owner.ClassMeta != nil)
	if !isIteratorKey {
		return false
	}
	if owner.IsPropertyOptional(marker) {
		return c.strictNullChecks
	}
	fn, ok := member.(*types.ObjectType)
	if !ok || len(fn.CallSigs()) != 1 {
		return false
	}
	ret, ok := types.GetEffectiveType(fn.CallSigs()[0].ReturnType).(*types.ObjectType)
	if !ok || ret.IsCallable() || len(ret.IndexSigs()) > 0 {
		return false
	}
	for _, base := range ret.Bases() {
		if _, known := base.(*types.ObjectType); !known {
			return false
		}
	}
	_, hasNext := ret.EffectiveProperty("next")
	return !hasNext
}

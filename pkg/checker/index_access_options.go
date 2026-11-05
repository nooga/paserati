package checker

import (
	"fmt"
	"strings"

	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// SetNoPropertyAccessFromIndexSignature mirrors
// --noPropertyAccessFromIndexSignature: `obj.name` that only an index
// signature provides is TS4111 (`obj["name"]` is required).
func (c *Checker) SetNoPropertyAccessFromIndexSignature(enabled bool) {
	c.noPropertyAccessFromIndexSignature = enabled
}

// checkPropertyAccessFromIndexSignature reports TS4111 for a property access
// `object.name` (plain or optional) whose property comes from an index
// signature of the object's type, as tsc's checkPropertyAccessExpressionOrQualifiedName
// does under --noPropertyAccessFromIndexSignature.
func (c *Checker) checkPropertyAccessFromIndexSignature(object parser.Expression, property parser.Expression) {
	if !c.noPropertyAccessFromIndexSignature || object == nil {
		return
	}
	ident, ok := property.(*parser.Identifier)
	if !ok || strings.HasPrefix(ident.Value, "#") {
		return
	}
	objectType := object.GetComputedType()
	if objectType == nil {
		// FIXME: a narrowed member access does not visit its object, so
		// its type is not known here.
		return
	}
	if c.propertyFromIndexSignatureOnly(objectType, ident.Value, 0) {
		c.addErrorWithCode(ident, "TS4111", fmt.Sprintf("Property '%s' comes from an index signature, so it must be accessed with ['%s'].", ident.Value, ident.Value))
	}
}

// propertyFromIndexSignatureOnly reports whether reading name from a value
// of type t finds no declared property but an applicable string index
// signature. For a union, no member may declare the property (tsc then
// makes no union property and falls back to the union's index signature);
// null and undefined members are skipped, as an optional chain or an
// earlier error does.
func (c *Checker) propertyFromIndexSignatureOnly(t types.Type, name string, depth int) bool {
	if t == nil || depth > 8 {
		return false
	}
	t = c.apparentType(c.rel.GetWidenedType(t))
	if ro, ok := t.(*types.ReadonlyType); ok {
		t = c.rel.GetWidenedType(ro.InnerType)
	}
	if inst, ok := t.(*types.InstantiatedType); ok {
		if body := inst.Substitute(); body != nil {
			t = body
		}
	}
	switch tt := t.(type) {
	case *types.UnionType:
		seen := false
		for _, m := range tt.Types {
			if m == types.Null || m == types.Undefined {
				continue
			}
			if !c.propertyFromIndexSignatureOnly(m, name, depth+1) {
				return false
			}
			seen = true
		}
		return seen
	case *types.ObjectType:
		if _, found := tt.EffectiveProperty(name); found {
			return false
		}
		if tt.IsCallable() || tt.IsConstructable() {
			if c.env.GetPrimitivePrototypeMethodType("function", name) != nil {
				return false
			}
		}
		// Members of the Object interface (its methods and `constructor`)
		// are properties of every object type.
		if name == "constructor" || c.env.GetPrimitivePrototypeMethodType("object", name) != nil {
			return false
		}
		for _, sig := range tt.IndexSigs() {
			if sig.Permissive || sig.Synthetic {
				continue
			}
			if sig.KeyType == types.String || sig.KeyType == types.Any {
				// FIXME: a property-less index-signature type of any, or of a union of functions
				// and generic aliases, is mostly an
				// imprecise stand-in for a generic type we did not resolve
				// (tRPC contexts); tsc has real members there, so stay quiet.
				if len(tt.Properties) == 0 && c.looksUnresolvedIndexValue(sig.ValueType) {
					continue
				}
				return true
			}
		}
	}
	return false
}

// looksUnresolvedIndexValue reports whether an index signature's value type
// is any or a union mixing function types, which is how an unresolved
// generic record (a mapped type over unknown keys) shows up here.
func (c *Checker) looksUnresolvedIndexValue(v types.Type) bool {
	if v == types.Any {
		return true
	}
	u, ok := v.(*types.UnionType)
	if !ok {
		return false
	}
	for _, m := range u.Types {
		if o, ok := m.(*types.ObjectType); ok && o.IsCallable() {
			return true
		}
	}
	return false
}

package checker

import (
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// discriminantAlias records that a const was destructured from a property of
// another binding: `const { kind } = shape`. Testing the alias
// (`kind === "circle"`) narrows shape by that discriminant, as tsc does for
// destructured discriminants.
type discriminantAlias struct {
	base string // the binding the alias was destructured from
	prop string // the property it names
}

// recordDestructuredDiscriminants notes each plain `const { p } = ident`
// (ident a const, a parameter or a local that is not reassigned in between,
// which is not checked) or
// `const { p: alias } = ident` binding of node (no default, no nesting).
func (c *Checker) recordDestructuredDiscriminants(node *parser.ObjectDestructuringDeclaration) {
	if node == nil || !node.IsConst {
		return
	}
	source, ok := node.Value.(*parser.Identifier)
	if !ok {
		return
	}
	if _, _, found := c.env.Resolve(source.Value); !found {
		return
	}
	for _, prop := range node.Properties {
		if prop == nil || prop.Default != nil {
			continue
		}
		key, ok := prop.Key.(*parser.Identifier)
		if !ok {
			continue
		}
		target, ok := prop.Target.(*parser.Identifier)
		if !ok || !c.env.HasLocalSymbol(target.Value) {
			continue
		}
		if c.env.discriminantAliases == nil {
			c.env.discriminantAliases = map[string]discriminantAlias{}
		}
		c.env.discriminantAliases[target.Value] = discriminantAlias{base: source.Value, prop: key.Value}
	}
}

// destructuredDiscriminantGuard is the guard `alias === value` implies for the
// object alias was destructured from, when that is a union. nil otherwise.
func (c *Checker) destructuredDiscriminantGuard(ident *parser.Identifier, value types.Type, negated bool) *TypeGuard {
	for e := c.env; e != nil; e = e.outer {
		if _, declared := e.symbols[ident.Value]; !declared {
			continue
		}
		alias, ok := e.discriminantAliases[ident.Value]
		if !ok {
			return nil
		}
		baseType, _, found := c.env.Resolve(alias.base)
		if !found {
			return nil
		}
		if _, isUnion := baseType.(*types.UnionType); !isUnion {
			return nil
		}
		return &TypeGuard{
			VariableName:      alias.base,
			IsNegated:         negated,
			DiscriminantProp:  alias.prop,
			DiscriminantValue: value,
		}
	}
	return nil
}

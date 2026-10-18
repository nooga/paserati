package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// checkSpreadOverrides reports TS2783 (tsc's checkSpreadPropOverrides): under
// strictNullChecks, a property written before a spread whose type has the
// same property, not optional, is always overwritten.
func (c *Checker) checkSpreadOverrides(node *parser.ObjectLiteral) {
	if !c.strictNullChecks || node == nil {
		return
	}
	if c.checkedSpreadOverrides == nil {
		c.checkedSpreadOverrides = make(map[*parser.ObjectLiteral]bool)
	}
	if c.checkedSpreadOverrides[node] {
		return
	}
	c.checkedSpreadOverrides[node] = true

	written := map[string]parser.Node{}
	var order []string
	write := func(name string, at parser.Node) {
		if _, seen := written[name]; !seen {
			order = append(order, name)
		}
		written[name] = at
	}
	for _, prop := range node.Properties {
		if prop == nil {
			continue
		}
		switch key := prop.Key.(type) {
		case *parser.Identifier:
			write(key.Value, key)
		case *parser.StringLiteral:
			write(key.Value, key)
		case *parser.SpreadElement:
			if key.Argument == nil {
				continue
			}
			t := key.Argument.GetComputedType()
			if t == nil {
				continue
			}
			for _, name := range order {
				if c.spreadAlwaysHas(t, name) {
					c.addErrorWithCode(written[name], errors.TS2783, fmt.Sprintf("'%s' is specified more than once, so this usage will be overwritten.", name))
				}
			}
		}
	}
}

// spreadAlwaysHas reports whether spreading a value of type t always
// writes property name: every member of t declares it, not optionally.
func (c *Checker) spreadAlwaysHas(t types.Type, name string) bool {
	t = c.resolveStructural(t)
	if u, ok := t.(*types.UnionType); ok {
		for _, m := range u.Types {
			if !c.spreadAlwaysHas(m, name) {
				return false
			}
		}
		return len(u.Types) > 0
	}
	obj, ok := t.(*types.ObjectType)
	if !ok || obj.IsClassConstructor() {
		return false
	}
	if _, has := obj.EffectiveProperty(name); !has {
		return false
	}
	return !obj.IsPropertyOptional(name)
}

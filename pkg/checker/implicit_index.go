package checker

import (
	"fmt"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
	"github.com/nooga/paserati/pkg/vm"
)

// reportImplicitAnyIndex is tsc's noImplicitAny report for `obj["name"]`
// naming no member of an object type without index signatures (the access
// is any): a close member name is suggested (TS2551), otherwise TS7053. An
// object literal indexed directly has no such member at all (TS2339).
func (c *Checker) reportImplicitAnyIndex(node *parser.IndexExpression, base *types.ObjectType, objectText, name string, indexType types.Type) {
	if !c.noImplicitAny || len(base.IndexSigs()) > 0 {
		return
	}
	if _, exists := c.resolveObjectMemberForDestructuring(base, name); exists {
		return
	}
	if _, isLiteral := node.Left.(*parser.ObjectLiteral); isLiteral {
		c.addErrorWithCode(node, errors.TS2339, fmt.Sprintf("Property '%s' does not exist on type '%s'.", name, objectText))
		return
	}
	if suggestion := getSpellingSuggestion(name, c.memberNames(base)); suggestion != "" {
		c.addErrorWithCode(node.Index, errors.TS2551, fmt.Sprintf("Property '%s' does not exist on type '%s'. Did you mean '%s'?", name, objectText, suggestion))
		return
	}
	c.addErrorWithCode(node, "TS7053", fmt.Sprintf("Element implicitly has an 'any' type because expression of type '%s' can't be used to index type '%s'.\n  Property '%s' does not exist on type '%s'.", indexType.String(), objectText, name, objectText))
}

// jsNumberKey is the property name a number denotes, its JavaScript ToString
// (Infinity, 1e-7, 1e+21).
func jsNumberKey(f float64) string {
	return vm.NumberValue(f).ToString()
}

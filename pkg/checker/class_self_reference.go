package checker

import (
	"fmt"
	"strings"

	"github.com/nooga/paserati/pkg/parser"
)

// reportClassUseInOwnComputedName reports TS2449 for references to a class
// from a computed member name of that class (`class A { [A.p]() {} }`).
// Computed keys are evaluated before the class binding is initialized, so
// tsc's isBlockScopedNameDeclaredBeforeUse rejects them even though the
// declaration precedes the use textually.
func (c *Checker) reportClassUseInOwnComputedName(expr parser.Expression, className string) {
	if className == "" || strings.HasPrefix(className, "__AnonymousClass_") {
		return
	}
	var walk func(n parser.Node)
	walk = func(n parser.Node) {
		if isNilNode(n) {
			return
		}
		switch t := n.(type) {
		case *parser.Identifier:
			if t.Value == className {
				c.addErrorWithCode(t, tsClassUsedBefore, fmt.Sprintf("Class '%s' used before its declaration.", className))
			}
			return
		case *parser.MemberExpression:
			// The property name is not a reference.
			walk(t.Object)
			return
		case *parser.ObjectLiteral:
			for _, p := range t.Properties {
				if p == nil {
					continue
				}
				if _, computed := p.Key.(*parser.ComputedPropertyName); computed {
					walk(p.Key)
				}
				walk(p.Value)
			}
			return
		case *parser.FunctionLiteral:
			if paramsDeclare(t.Parameters, className) {
				return
			}
		case *parser.ArrowFunctionLiteral:
			if paramsDeclare(t.Parameters, className) {
				return
			}
		case *parser.ClassExpression, *parser.ClassDeclaration:
			// A nested class has computed keys of its own.
			return
		}
		forEachChildInScope(n, walk)
	}
	walk(expr)
}

// paramsDeclare reports whether a simple parameter named name shadows it.
func paramsDeclare(params []*parser.Parameter, name string) bool {
	for _, p := range params {
		if p != nil && p.Name != nil && p.Name.Value == name {
			return true
		}
	}
	return false
}

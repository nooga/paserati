package checker

import (
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// Aliased conditions (TypeScript 4.4): after `const ok = id !== undefined`,
// `if (ok)` narrows `id` the way the condition itself would, as long as what
// the condition tests cannot have changed in between: const variables and
// parameters that are never assigned.

// collectConstAliases maps each unannotated `const name = init` in the
// program to its initializer. A name declared by more than one const is
// ambiguous without scope resolution and is left out.
func collectConstAliases(program *parser.Program) map[string]parser.Expression {
	found := map[string]parser.Expression{}
	ambiguous := map[string]bool{}
	forEachDescendant(program, func(n parser.Node) {
		stmt, ok := n.(*parser.ConstStatement)
		if !ok {
			return
		}
		for _, d := range stmt.Declarations {
			if d == nil || d.Name == nil || d.Value == nil || d.TypeAnnotation != nil {
				continue
			}
			if _, dup := found[d.Name.Value]; dup {
				ambiguous[d.Name.Value] = true
			}
			found[d.Name.Value] = d.Value
		}
	})
	for name := range ambiguous {
		delete(found, name)
	}
	return found
}

// stableParameters are the parameters a function body never assigns.
func stableParameters(params []*parser.Parameter, body parser.Node) map[string]bool {
	stable := map[string]bool{}
	written := map[string]bool{}
	for _, key := range writtenRefs(body) {
		written[key] = true
	}
	for _, p := range params {
		if p != nil && p.Name != nil && !written[p.Name.Value] {
			stable[p.Name.Value] = true
		}
	}
	return stable
}

// conditionReferencesStable reports whether every variable the condition
// reads is one an alias may rely on.
func (c *Checker) conditionReferencesStable(expr parser.Expression) bool {
	stable := true
	forEachDescendant(expr, func(n parser.Node) {
		id, ok := n.(*parser.Identifier)
		if !ok || !stable {
			return
		}
		t, isConst, found := c.env.Resolve(id.Value)
		if !found || isConst || c.stableParams[id.Value] {
			return
		}
		if obj, ok := t.(*types.ObjectType); ok && obj.IsCallable() {
			return // a function declaration
		}
		stable = false
	})
	return stable
}

// resolveAliasedCondition replaces `ok` (or `!ok`) by the condition `ok` was
// initialised with when it can be relied on, and otherwise returns cond.
func (c *Checker) resolveAliasedCondition(cond parser.Expression) parser.Expression {
	return c.resolveAliasedConditionDepth(cond, 0)
}

func (c *Checker) resolveAliasedConditionDepth(cond parser.Expression, depth int) parser.Expression {
	if depth > 4 || len(c.constAliases) == 0 {
		return cond
	}
	switch e := cond.(type) {
	case *parser.Identifier:
		_, isConst, found := c.env.Resolve(e.Value)
		if !found || !isConst {
			return cond
		}
		init, ok := c.constAliases[e.Value]
		if !ok || !c.conditionReferencesStable(init) {
			return cond
		}
		return c.resolveAliasedConditionDepth(init, depth+1)
	case *parser.PrefixExpression:
		if e.Operator != "!" {
			return cond
		}
		if _, ok := e.Right.(*parser.Identifier); !ok {
			return cond
		}
		inner := c.resolveAliasedConditionDepth(e.Right, depth+1)
		if inner == e.Right {
			return cond
		}
		return &parser.PrefixExpression{Token: e.Token, Operator: "!", Right: inner}
	}
	return cond
}

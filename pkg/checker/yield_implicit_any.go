package checker

import (
	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// tsc types a yield expression's result by the generator's contextual next
// type. With none (no return type annotation) the result is any, and under
// noImplicitAny a yield whose result is used and has no contextual type of
// its own is TS7057.

// markUnusedResult records the yield expressions whose value expr discards:
// expr itself, both sides of a comma, the operand of void.
func (c *Checker) markUnusedResult(expr parser.Expression) {
	switch e := expr.(type) {
	case *parser.YieldExpression:
		if c.unusedYields == nil {
			c.unusedYields = make(map[*parser.YieldExpression]bool)
		}
		c.unusedYields[e] = true
	case *parser.InfixExpression:
		if e.Operator == "," {
			c.markUnusedResult(e.Left)
			c.markUnusedResult(e.Right)
		}
	case *parser.PrefixExpression:
		if e.Operator == "void" {
			c.markUnusedResult(e.Right)
		}
	}
}

// markContextualYield records a yield whose position gives it a type.
func (c *Checker) markContextualYield(expr parser.Expression) {
	if y, ok := expr.(*parser.YieldExpression); ok {
		if c.contextualYields == nil {
			c.contextualYields = make(map[*parser.YieldExpression]bool)
		}
		c.contextualYields[y] = true
	}
}

// markArgumentGenerators records generator function arguments: the call's
// parameter types give them a contextual return type.
func (c *Checker) markArgumentGenerators(args []parser.Expression) {
	for _, arg := range args {
		if fn, ok := arg.(*parser.FunctionLiteral); ok && fn.IsGenerator && fn.Body != nil {
			if c.argumentGenerators == nil {
				c.argumentGenerators = make(map[parser.Node]bool)
			}
			c.argumentGenerators[fn.Body] = true
		}
	}
}

// noteContextualYield records a yield visited with an expected type.
func (c *Checker) noteContextualYield(node parser.Node, expected types.Type) {
	y, ok := node.(*parser.YieldExpression)
	if !ok || expected == nil || expected == types.Any {
		return
	}
	if _, isParam := expected.(*types.TypeParameterType); isParam {
		return // an uninferred type parameter gives no type
	}
	if c.contextualYields == nil {
		c.contextualYields = make(map[*parser.YieldExpression]bool)
	}
	c.contextualYields[y] = true
}

func (c *Checker) checkImplicitAnyYield(node *parser.YieldExpression) {
	if !c.noImplicitAny || node.Delegate || c.generatorNextTyped || c.unusedYields[node] || c.contextualYields[node] {
		return
	}
	c.addErrorWithCode(node, errors.TS7057, "'yield' expression implicitly results in an 'any' type because its containing generator lacks a return-type annotation.")
}

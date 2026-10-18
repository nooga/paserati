package checker

import (
	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
)

// truthySemantics mirrors tsc's PredicateSemantics: whether an expression,
// judged by its syntax alone, is always, never or only sometimes truthy.
type truthySemantics int

const (
	truthyAlways    truthySemantics = 1
	truthyNever     truthySemantics = 2
	truthySometimes                 = truthyAlways | truthyNever
)

// syntacticTruthySemantics is tsc's getSyntacticTruthySemantics.
func syntacticTruthySemantics(node parser.Expression) truthySemantics {
	for {
		switch n := node.(type) {
		case *parser.TypeAssertionExpression:
			node = n.Expression
			continue
		case *parser.SatisfiesExpression:
			node = n.Expression
			continue
		case *parser.NonNullExpression:
			node = n.Expression
			continue
		}
		break
	}
	if isNilNode(node) {
		return truthySometimes
	}
	switch n := node.(type) {
	case *parser.NumberLiteral:
		// `while (0)` and `while (1)` are idioms.
		if n.Token != nil && (n.Token.Literal == "0" || n.Token.Literal == "1") {
			return truthySometimes
		}
		return truthyAlways
	case *parser.ArrayLiteral, *parser.ArrowFunctionLiteral, *parser.BigIntLiteral,
		*parser.ClassExpression, *parser.FunctionLiteral, *parser.ObjectLiteral, *parser.RegexLiteral:
		return truthyAlways
	case *parser.NullLiteral, *parser.UndefinedLiteral:
		return truthyNever
	case *parser.PrefixExpression:
		if n.Operator == "void" {
			return truthyNever
		}
	case *parser.StringLiteral:
		if n.Value != "" {
			return truthyAlways
		}
		return truthyNever
	case *parser.TemplateLiteral:
		text := ""
		for _, part := range n.Parts {
			sp, ok := part.(*parser.TemplateStringPart)
			if !ok {
				return truthySometimes
			}
			text += sp.Value
		}
		if text != "" {
			return truthyAlways
		}
		return truthyNever
	case *parser.TernaryExpression:
		return syntacticTruthySemantics(n.Consequence) | syntacticTruthySemantics(n.Alternative)
	case *parser.Identifier:
		if n.Value == "undefined" {
			return truthyNever
		}
	}
	return truthySometimes
}

// checkSyntacticTruthiness reports conditions whose truthiness their syntax
// already decides (TS2872, TS2873), as tsc's checkTruthinessOfType does for
// if/while/do/for conditions, `?:`, `!` and the left side of `&&` and `||`.
func (c *Checker) checkSyntacticTruthiness(program *parser.Program) {
	test := func(e parser.Expression) {
		switch syntacticTruthySemantics(e) {
		case truthyAlways:
			c.addErrorAtStart(e, errors.TS2872, "This kind of expression is always truthy.")
		case truthyNever:
			c.addErrorAtStart(e, errors.TS2873, "This kind of expression is always falsy.")
		}
	}
	var walk func(n parser.Node)
	walk = func(n parser.Node) {
		switch x := n.(type) {
		case *parser.IfStatement:
			test(x.Condition)
		case *parser.WhileStatement:
			test(x.Condition)
		case *parser.DoWhileStatement:
			test(x.Condition)
		case *parser.ForStatement:
			if !isNilNode(x.Condition) {
				test(x.Condition)
			}
		case *parser.TernaryExpression:
			test(x.Condition)
		case *parser.PrefixExpression:
			if x.Operator == "!" {
				test(x.Right)
			}
		case *parser.InfixExpression:
			if x.Operator == "&&" || x.Operator == "||" {
				test(x.Left)
			}
		}
		forEachChildInScope(n, walk)
	}
	walk(program)
}

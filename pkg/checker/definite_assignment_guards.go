package checker

import (
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// Narrowing for definite assignment. tsc computes TS2454 from the flow type of
// a reference: a read whose narrowed type no longer admits `undefined` is not
// "used before being assigned", even if no write reached it. So a guard that
// rules out undefined on one branch (`typeof x === "number"`, `if (x)`,
// `x !== undefined`, `x instanceof C`, a type predicate) counts as an
// assignment on that branch. Likewise a branch that a literal `true`/`false`
// condition makes unreachable is not checked at all.

// constantCondition evaluates a condition the way tsc's binder does when it
// builds flow conditions: only the `true`/`false` keywords are constant, seen
// through `!`, `&&` and `||`.
func constantCondition(e parser.Expression) (value, ok bool) {
	switch n := e.(type) {
	case *parser.BooleanLiteral:
		return n.Value, true
	case *parser.PrefixExpression:
		if n.Operator == "!" {
			v, ok := constantCondition(n.Right)
			return !v, ok
		}
	case *parser.InfixExpression:
		l, lok := constantCondition(n.Left)
		switch n.Operator {
		case "&&":
			if lok && !l {
				return false, true
			}
			if lok {
				return constantCondition(n.Right)
			}
		case "||":
			if lok && l {
				return true, true
			}
			if lok {
				return constantCondition(n.Right)
			}
		}
	}
	return false, false
}

// conditionGuards returns the candidate names a condition proves defined when
// it is true and when it is false.
func (a *definiteAssignment) conditionGuards(e parser.Expression) (whenTrue, whenFalse map[string]bool) {
	whenTrue, whenFalse = map[string]bool{}, map[string]bool{}
	a.collectGuards(e, whenTrue, whenFalse)
	return
}

func (a *definiteAssignment) collectGuards(e parser.Expression, whenTrue, whenFalse map[string]bool) {
	switch n := e.(type) {
	case *parser.Identifier:
		// Truthiness rules out undefined.
		if a.candidates[n.Value] {
			whenTrue[n.Value] = true
		}
	case *parser.PrefixExpression:
		if n.Operator == "!" {
			a.collectGuards(n.Right, whenFalse, whenTrue)
		}
	case *parser.InfixExpression:
		switch n.Operator {
		case "&&":
			lt, lf := a.conditionGuards(n.Left)
			rt, rf := a.conditionGuards(n.Right)
			union(whenTrue, lt, rt)
			intersect(whenFalse, lf, rf)
		case "||":
			lt, lf := a.conditionGuards(n.Left)
			rt, rf := a.conditionGuards(n.Right)
			intersect(whenTrue, lt, rt)
			union(whenFalse, lf, rf)
		case "===", "==", "!==", "!=":
			eqTrue, eqFalse := whenTrue, whenFalse
			if n.Operator == "!==" || n.Operator == "!=" {
				eqTrue, eqFalse = whenFalse, whenTrue
			}
			a.equalityGuards(n.Left, n.Right, n.Operator, eqTrue, eqFalse)
			a.equalityGuards(n.Right, n.Left, n.Operator, eqTrue, eqFalse)
		case "instanceof":
			if id, ok := n.Left.(*parser.Identifier); ok && a.candidates[id.Value] {
				whenTrue[id.Value] = true
			}
		case "in":
			if id, ok := n.Right.(*parser.Identifier); ok && a.candidates[id.Value] {
				whenTrue[id.Value] = true
			}
		}
	case *parser.CallExpression:
		a.predicateGuards(n, whenTrue)
	}
}

// equalityGuards handles `ref == other` (whenEqual/whenNotEqual are already
// swapped for != and !==).
func (a *definiteAssignment) equalityGuards(ref, other parser.Expression, op string, whenEqual, whenNotEqual map[string]bool) {
	if t, ok := ref.(*parser.TypeofExpression); ok {
		id, ok := t.Operand.(*parser.Identifier)
		lit, isLit := other.(*parser.StringLiteral)
		if !ok || !isLit || !a.candidates[id.Value] {
			return
		}
		if lit.Value == "undefined" {
			whenNotEqual[id.Value] = true
		} else {
			whenEqual[id.Value] = true
		}
		return
	}
	id, ok := ref.(*parser.Identifier)
	if !ok || !a.candidates[id.Value] {
		return
	}
	loose := op == "==" || op == "!="
	switch o := other.(type) {
	case *parser.UndefinedLiteral:
		whenNotEqual[id.Value] = true
	case *parser.NullLiteral:
		if loose {
			whenNotEqual[id.Value] = true
		} else {
			// Narrowed to null on the equal branch: no undefined left.
			whenEqual[id.Value] = true
		}
	case *parser.StringLiteral, *parser.NumberLiteral, *parser.BooleanLiteral, *parser.TemplateLiteral:
		_ = o
		whenEqual[id.Value] = true
	}
}

// predicateGuards marks the argument a type predicate (`x is T` with a T that
// excludes undefined) narrows on the true branch.
func (a *definiteAssignment) predicateGuards(call *parser.CallExpression, whenTrue map[string]bool) {
	if call.Function == nil {
		return
	}
	if a.collectOnly {
		// Before checking there are no types to find predicates by, so any
		// argument may be guarded.
		for _, arg := range call.Arguments {
			if id, ok := arg.(*parser.Identifier); ok && a.candidates[id.Value] {
				whenTrue[id.Value] = true
			}
		}
		return
	}
	ft, ok := call.Function.GetComputedType().(*types.ObjectType)
	if !ok {
		return
	}
	for _, sig := range ft.CallSigs() {
		pred, ok := sig.ReturnType.(*types.TypePredicateType)
		if !ok || pred.Asserts || pred.Type == nil || strictInitTypePermits(pred.Type) {
			continue
		}
		i := predicateArgumentIndex(&typePredicateSignature{predicate: pred, signature: sig}, len(call.Arguments))
		if i < 0 {
			continue
		}
		if id, ok := call.Arguments[i].(*parser.Identifier); ok && a.candidates[id.Value] {
			whenTrue[id.Value] = true
		}
	}
}

func union(dst map[string]bool, srcs ...map[string]bool) {
	for _, s := range srcs {
		for k := range s {
			dst[k] = true
		}
	}
}

func intersect(dst, x, y map[string]bool) {
	for k := range x {
		if y[k] {
			dst[k] = true
		}
	}
}

// withGuards returns a copy of assigned with the guarded names added.
func withGuards(assigned, guards map[string]bool) map[string]bool {
	out := copyAssigned(assigned)
	for k := range guards {
		out[k] = true
	}
	return out
}

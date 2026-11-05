package checker

import (
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// SetNoImplicitReturns mirrors --noImplicitReturns (TS7030).
func (c *Checker) SetNoImplicitReturns(enabled bool) {
	c.noImplicitReturns = enabled
}

// SetNoFallthroughCasesInSwitch mirrors --noFallthroughCasesInSwitch
// (TS7029).
func (c *Checker) SetNoFallthroughCasesInSwitch(enabled bool) {
	c.noFallthroughCasesInSwitch = enabled
}

const notAllCodePathsReturn = "Not all code paths return a value."

// checkImplicitReturns is the --noImplicitReturns part of tsc's
// checkAllCodePathsInNonVoidFunctionReturnOrThrow: a function whose end is
// reachable, that returns a value somewhere, and whose return type is not
// void, any or undefined, reports TS7030 on its return type annotation (or
// on the function when it has none). The cases TS2355/TS2366 cover are left
// to checkMissingReturn. declared is the annotated return type, inferred the
// type inferred from the body when there is no annotation.
func (c *Checker) checkImplicitReturns(errorNode parser.Node, annotation parser.Expression, declared, inferred types.Type, body *parser.BlockStatement, isAsync, isGenerator bool) {
	if !c.noImplicitReturns || body == nil || isGenerator || isNilNode(errorNode) {
		return
	}
	// FIXME: generators (their TReturn) are not checked.
	if !hasAnyReturn(body) || !c.endReachable(body) {
		return
	}
	if annotation != nil {
		if declared == nil {
			return
		}
		t := declared
		if isAsync {
			t = c.getAwaitedType(t)
		}
		if pred, ok := t.(*types.TypePredicateType); ok {
			if pred.Asserts {
				return
			}
			t = types.Boolean
		}
		if c.returnTypeUndefinedVoidOrAny(t) || t == types.Never {
			return
		}
		// Without undefined in the return type it is TS2366's case.
		if c.strictNullChecks && !c.rel.IsAssignable(types.Undefined, t) {
			return
		}
		c.addErrorWithCode(annotation, "TS7030", notAllCodePathsReturn)
		return
	}
	if inferred == nil {
		return
	}
	t := inferred
	if isAsync {
		t = c.getAwaitedType(t)
	}
	if c.returnTypeUndefinedVoidOrAny(t) {
		return
	}
	c.addErrorWithCode(errorNode, "TS7030", notAllCodePathsReturn)
}

// returnTypeUndefinedVoidOrAny is tsc's isUnwrappedReturnTypeUndefinedVoidOrAny
// for an already unwrapped type: it may be void, or it is any or undefined.
// Types the checker cannot resolve here count as any.
func (c *Checker) returnTypeUndefinedVoidOrAny(t types.Type) bool {
	switch t {
	case nil, types.Any, types.Void, types.Undefined:
		return true
	}
	switch tt := t.(type) {
	case *types.UnionType:
		for _, m := range tt.Types {
			if m == types.Void {
				return true
			}
		}
	case *types.TypeParameterType, *types.ForwardReferenceType, *types.ParameterizedForwardReferenceType:
		return true // FIXME: cannot tell what it resolves to
	}
	return false
}

// hasAnyReturn reports whether the body contains a return statement, with
// or without a value, outside nested functions (tsc's HasExplicitReturn).
func hasAnyReturn(body parser.Node) bool {
	found := false
	walkStatements(body, func(s parser.Statement) {
		if _, ok := s.(*parser.ReturnStatement); ok {
			found = true
		}
	})
	return found
}

// checkSwitchFallthrough reports TS7029 on each case clause with statements,
// other than the last, whose end is reachable (--noFallthroughCasesInSwitch).
func (c *Checker) checkSwitchFallthrough(s *parser.SwitchStatement) {
	if !c.noFallthroughCasesInSwitch || s == nil {
		return
	}
	for i, cs := range s.Cases {
		if i == len(s.Cases)-1 || cs == nil || cs.Body == nil || len(cs.Body.Statements) == 0 {
			continue
		}
		if c.endReachable(cs.Body) {
			c.addErrorWithCode(&parser.Identifier{Token: cs.Token, Value: cs.Token.Literal}, "TS7029", "Fallthrough case in switch.")
		}
	}
}

// functionErrorNode is where tsc puts a diagnostic about a whole function
// literal: its name, else the function (its `function` keyword).
func functionErrorNode(fn *parser.FunctionLiteral) parser.Node {
	if fn == nil {
		return nil
	}
	if fn.Name != nil {
		return fn.Name
	}
	return fn
}

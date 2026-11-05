package checker

import (
	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// checkMissingReturn reports TS2355 / TS2366 for a function whose declared
// return type excludes undefined but whose body can run off its end. The
// reachability analysis is deliberately conservative: where it cannot tell
// (exhaustive switches over non-literal types, calls it cannot prove never
// return), the end counts as unreachable and nothing is reported.
func (c *Checker) checkMissingReturn(annotation parser.Expression, declared types.Type, body *parser.BlockStatement, isAsync, isGenerator bool) {
	if annotation == nil || declared == nil || body == nil || isGenerator {
		return
	}
	if isAsync {
		declared = c.getAwaitedType(declared)
	}
	if _, isPredicate := declared.(*types.TypePredicateType); isPredicate {
		return
	}
	if c.returnTypeAllowsFallOff(declared) {
		return
	}
	if !c.endReachable(body) {
		return
	}
	if !hasReturnWithValue(body) {
		c.addErrorWithCode(annotation, errors.TS2355,
			"A function whose declared type is neither 'undefined', 'void', nor 'any' must return a value.")
		return
	}
	c.addErrorWithCode(annotation, errors.TS2366,
		"Function lacks ending return statement and return type does not include 'undefined'.")
}

// returnTypeAllowsFallOff reports whether a function returning t may finish
// without a return statement.
func (c *Checker) returnTypeAllowsFallOff(t types.Type) bool {
	switch t {
	case types.Any, types.Unknown, types.Void, types.Undefined, types.Never:
		return true
	}
	if !c.rel.StrictNullChecks {
		return true
	}
	switch tt := t.(type) {
	case *types.UnionType:
		for _, m := range tt.Types {
			if c.returnTypeAllowsFallOff(m) {
				return true
			}
		}
		return false
	case *types.TypeParameterType, *types.ForwardReferenceType, *types.ConditionalType, *types.IndexedAccessType:
		return true // cannot tell what it resolves to
	}
	return false
}

// hasReturnWithValue reports whether the body contains `return <expr>`,
// ignoring nested functions.
func hasReturnWithValue(node parser.Node) bool {
	found := false
	walkStatements(node, func(s parser.Statement) {
		if r, ok := s.(*parser.ReturnStatement); ok && r.ReturnValue != nil {
			found = true
		}
	})
	return found
}

// walkStatements visits every statement under node (not entering functions).
func walkStatements(node parser.Node, visit func(parser.Statement)) {
	var walk func(n parser.Node)
	walk = func(n parser.Node) {
		switch s := n.(type) {
		case *parser.BlockStatement:
			if s == nil {
				return
			}
			for _, st := range s.Statements {
				visit(st)
				walk(st)
			}
		case *parser.IfStatement:
			walk(s.Consequence)
			if s.Alternative != nil {
				walk(s.Alternative)
			}
		case *parser.WhileStatement:
			walk(s.Body)
		case *parser.DoWhileStatement:
			walk(s.Body)
		case *parser.ForStatement:
			walk(s.Body)
		case *parser.ForInStatement:
			walk(s.Body)
		case *parser.ForOfStatement:
			walk(s.Body)
		case *parser.LabeledStatement:
			visit(s.Statement)
			walk(s.Statement)
		case *parser.SwitchStatement:
			for _, cs := range s.Cases {
				walk(cs.Body)
			}
		case *parser.TryStatement:
			walk(s.Body)
			if s.CatchClause != nil {
				walk(s.CatchClause.Body)
			}
			if s.FinallyBlock != nil {
				walk(s.FinallyBlock)
			}
		}
	}
	walk(node)
}

// endReachable reports whether control can reach the end of stmt.
func (c *Checker) endReachable(stmt parser.Node) bool {
	switch s := stmt.(type) {
	case nil:
		return true
	case *parser.ReturnStatement, *parser.ThrowStatement, *parser.BreakStatement, *parser.ContinueStatement:
		return false
	case *parser.BlockStatement:
		if s == nil {
			return true
		}
		for _, st := range s.Statements {
			if !c.endReachable(st) {
				return false
			}
		}
		return true
	case *parser.ExpressionStatement:
		if call, ok := s.Expression.(*parser.CallExpression); ok && call.GetComputedType() == types.Never {
			return false
		}
		return true
	case *parser.IfStatement:
		if s.Alternative == nil {
			return true
		}
		return c.endReachable(s.Consequence) || c.endReachable(s.Alternative)
	case *parser.WhileStatement:
		if isTrueLiteral(s.Condition) {
			return breaksOut(s.Body, "")
		}
		return true
	case *parser.ForStatement:
		if s.Condition == nil || isTrueLiteral(s.Condition) {
			return breaksOut(s.Body, "")
		}
		return true
	case *parser.DoWhileStatement:
		if isTrueLiteral(s.Condition) {
			return breaksOut(s.Body, "")
		}
		if c.endReachable(s.Body) || containsContinue(s.Body) {
			return true
		}
		return breaksOut(s.Body, "")
	case *parser.LabeledStatement:
		return c.endReachable(s.Statement) || breaksOut(s.Statement, s.Label.Value)
	case *parser.SwitchStatement:
		return c.switchEndReachable(s)
	case *parser.TryStatement:
		if s.FinallyBlock != nil && !c.endReachable(s.FinallyBlock) {
			return false
		}
		if c.endReachable(s.Body) {
			return true
		}
		return s.CatchClause != nil && c.endReachable(s.CatchClause.Body)
	}
	return true
}

func (c *Checker) switchEndReachable(s *parser.SwitchStatement) bool {
	hasDefault := false
	for _, cs := range s.Cases {
		if cs.Condition == nil {
			hasDefault = true
		}
	}
	if !hasDefault && !c.switchIsExhaustive(s) {
		return true
	}
	for i, cs := range s.Cases {
		if breaksOutOfSwitch(cs.Body) {
			return true
		}
		if i == len(s.Cases)-1 && c.endReachable(cs.Body) {
			return true
		}
	}
	return len(s.Cases) == 0
}

// switchIsExhaustive reports whether a switch without a default covers every
// member of its expression's type, as tsc's computeExhaustiveSwitchStatement
// does: the type must be a unit type or a union of them (literals, true and
// false, enum members, null, undefined), every case value a unit type, and
// every member of the type among the case values. Anything else is not
// claimed exhaustive.
func (c *Checker) switchIsExhaustive(s *parser.SwitchStatement) bool {
	if s.Expression == nil {
		return false
	}
	// FIXME: indexing or destructuring a union of tuples yields any here, not
	// the element union; claiming such a switch exhaustive avoids a false
	// TS7030 at the cost of missing a true one on a genuine any.
	if st := s.Expression.GetComputedType(); st == nil || st == types.Any {
		return true
	}
	members, ok := c.unitTypeMembers(s.Expression.GetComputedType(), 0)
	if !ok || len(members) == 0 {
		return false
	}
	var cases []types.Type
	for _, cs := range s.Cases {
		if cs == nil || cs.Condition == nil {
			continue
		}
		units, ok := c.unitTypeMembers(cs.Condition.GetComputedType(), 0)
		if !ok || len(units) != 1 {
			return false
		}
		cases = append(cases, units[0])
	}
	for _, m := range members {
		covered := false
		for _, ct := range cases {
			if sameUnitType(m, ct) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

// unitTypeMembers flattens t into the unit types it is a union of, with
// boolean as true | false and an enum as its members; ok is false when some
// member is not a unit type.
func (c *Checker) unitTypeMembers(t types.Type, depth int) ([]types.Type, bool) {
	if t == nil || depth > 8 {
		return nil, false
	}
	t = c.resolveTypeofTypeIfNeeded(t)
	t = c.resolveTypeAlias(t)
	switch tt := t.(type) {
	case *types.LiteralType:
		return []types.Type{tt}, true
	case *types.EnumMemberType:
		return []types.Type{tt}, true
	case *types.EnumType:
		var out []types.Type
		for _, name := range tt.OrderedMemberNames() {
			out = append(out, tt.Members[name])
		}
		return out, len(out) > 0
	case *types.UnionType:
		var out []types.Type
		for _, m := range tt.Types {
			units, ok := c.unitTypeMembers(m, depth+1)
			if !ok {
				return nil, false
			}
			out = append(out, units...)
		}
		return out, true
	}
	switch t {
	case types.Null, types.Undefined:
		return []types.Type{t}, true
	case types.Boolean:
		return []types.Type{types.TrueType, types.FalseType}, true
	}
	return nil, false
}

// sameUnitType reports whether two unit types are the same value.
func sameUnitType(a, b types.Type) bool {
	if a == b {
		return true
	}
	switch at := a.(type) {
	case *types.LiteralType:
		return at.Equals(b)
	case *types.EnumMemberType:
		bt, ok := b.(*types.EnumMemberType)
		return ok && at.EnumName == bt.EnumName && at.MemberName == bt.MemberName
	}
	return false
}

func isTrueLiteral(e parser.Expression) bool {
	b, ok := e.(*parser.BooleanLiteral)
	return ok && b.Value
}

// breaksOut reports whether node contains a break that leaves the enclosing
// loop (unlabeled, outside any nested loop or switch) or targets label.
func breaksOut(node parser.Node, label string) bool {
	return findJump(node, label, true, false)
}

func breaksOutOfSwitch(node parser.Node) bool {
	return findJump(node, "", true, false)
}

func containsContinue(node parser.Node) bool {
	return findJump(node, "", false, true)
}

// findJump looks for a break (wantBreak) or continue (wantContinue) that is
// bound to the construct containing node: unlabeled and not inside a nested
// loop/switch (a switch only captures break), or carrying the given label.
func findJump(node parser.Node, label string, wantBreak, wantContinue bool) bool {
	var scan func(n parser.Node, inLoop, inSwitch bool) bool
	scan = func(n parser.Node, inLoop, inSwitch bool) bool {
		switch s := n.(type) {
		case *parser.BreakStatement:
			if !wantBreak {
				return false
			}
			if s.Label != nil {
				return label != "" && s.Label.Value == label
			}
			return !inLoop && !inSwitch
		case *parser.ContinueStatement:
			if !wantContinue {
				return false
			}
			if s.Label != nil {
				return label != "" && s.Label.Value == label
			}
			return !inLoop
		case *parser.BlockStatement:
			if s == nil {
				return false
			}
			for _, st := range s.Statements {
				if scan(st, inLoop, inSwitch) {
					return true
				}
			}
		case *parser.IfStatement:
			return scan(s.Consequence, inLoop, inSwitch) || (s.Alternative != nil && scan(s.Alternative, inLoop, inSwitch))
		case *parser.WhileStatement:
			return scan(s.Body, true, inSwitch)
		case *parser.DoWhileStatement:
			return scan(s.Body, true, inSwitch)
		case *parser.ForStatement:
			return scan(s.Body, true, inSwitch)
		case *parser.ForInStatement:
			return scan(s.Body, true, inSwitch)
		case *parser.ForOfStatement:
			return scan(s.Body, true, inSwitch)
		case *parser.LabeledStatement:
			return scan(s.Statement, inLoop, inSwitch)
		case *parser.SwitchStatement:
			for _, cs := range s.Cases {
				if scan(cs.Body, inLoop, true) {
					return true
				}
			}
		case *parser.TryStatement:
			if scan(s.Body, inLoop, inSwitch) {
				return true
			}
			if s.CatchClause != nil && scan(s.CatchClause.Body, inLoop, inSwitch) {
				return true
			}
			return s.FinallyBlock != nil && scan(s.FinallyBlock, inLoop, inSwitch)
		}
		return false
	}
	return scan(node, false, false)
}

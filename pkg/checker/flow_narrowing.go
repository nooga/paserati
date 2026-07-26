package checker

import (
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// flowNarrowState tracks assignment-based flow narrowing for one straight-
// line statement sequence: a Program's top-level statements, or a single
// block's own statement list. It mirrors what TypeScript does with `let
// y = true; y && ...` — right after the assignment, `y`'s type at a read is
// the literal `true`, not the declared `boolean`, until something happens
// that this analysis can't follow through a branch.
//
// This is deliberately conservative, not full control-flow analysis: only a
// bare `name = expr` reassignment and a fresh let/var declaration keep
// narrowing alive. Any other statement (if, loop, switch, try, a function
// literal, or even just an expression this code doesn't specifically
// recognize) invalidates everything tracked so far, since there's no way to
// tell from here whether it reassigned a tracked variable through a branch
// that wasn't analyzed. That can only lose narrowing that TypeScript would
// have kept — never accept something TypeScript would reject — so it's safe
// to be aggressive about giving up.
//
// The overlay itself (Checker.flowNarrowOverlay) is a flat, unscoped map
// keyed by name, consulted only by identifier reads on top of the normal
// (and still authoritative) declared type in c.env. Each flowNarrowState
// instance is responsible for cleaning up exactly the entries it added
// before its statement sequence returns, so nested or sibling sequences
// with a shadowing name never see stale entries — see invalidateAll, called
// both mid-sequence (before an unrecognized statement) and once more at the
// end of the sequence.
type flowNarrowState struct {
	c       *Checker
	tracked map[string]bool
}

func (c *Checker) newFlowNarrowState() *flowNarrowState {
	return &flowNarrowState{c: c, tracked: make(map[string]bool)}
}

// track records that name's flow type is narrowType until something
// invalidates it.
func (f *flowNarrowState) track(name string, narrowType types.Type) {
	if f.c.flowNarrowOverlay == nil {
		f.c.flowNarrowOverlay = make(map[string]types.Type)
	}
	f.c.flowNarrowOverlay[name] = narrowType
	f.tracked[name] = true
}

// forget drops any narrowing this state holds for name, e.g. because it's
// about to be reassigned to something that isn't confidently narrowable, or
// because it's being read as an assignment target (which must see the
// declared type, not a stale narrow one).
func (f *flowNarrowState) forget(name string) {
	if !f.tracked[name] {
		return
	}
	delete(f.c.flowNarrowOverlay, name)
	delete(f.tracked, name)
}

// invalidateAll drops every narrowing this state holds, because a statement
// follows that this analysis can't linearly account for.
func (f *flowNarrowState) invalidateAll() {
	for name := range f.tracked {
		delete(f.c.flowNarrowOverlay, name)
	}
	f.tracked = make(map[string]bool)
}

// applyToStatement is called once per statement in a straight-line sequence,
// in order, after the checker has already fully processed stmt (visited it
// and, for a let/var, resolved and possibly widened its declared type).
// declaredType/narrowType/isFreshLiteralWiden describe a let/var statement's
// own outcome, already computed by the caller, so this doesn't need to
// re-derive checker state — see the call sites in checker.go.
func (f *flowNarrowState) observeLetOrVar(name string, declaredType, narrowType types.Type, widened bool) {
	if !widened {
		// The declared type already equals the initializer's own type (no
		// widening happened), so the overlay would add nothing.
		return
	}
	if _, isLiteral := narrowType.(*types.LiteralType); !isLiteral {
		return
	}
	f.track(name, narrowType)
}

// trackVarDeclarationNarrowing is called after a let/var/const statement's
// declarators have all been fully checked (so c.env holds each name's final
// declared type), to keep flow narrowing in sync for straight-line reads
// that follow within the same statement sequence. Unlike the Pass 5
// top-level path (see the LetStatement/ConstStatement/VarStatement case in
// Check), this doesn't have direct access to the widening decision, so it
// infers "did this widen" by comparing the declared type against the
// initializer's own computed type.
func (f *flowNarrowState) trackVarDeclarationNarrowing(c *Checker, declarators []*parser.VarDeclarator) {
	for _, declarator := range declarators {
		if declarator == nil || declarator.Name == nil || declarator.Value == nil || declarator.TypeAnnotation != nil {
			continue
		}
		narrowType, isLiteral := declarator.Value.GetComputedType().(*types.LiteralType)
		if !isLiteral {
			continue
		}
		declaredType, _, found := c.env.Resolve(declarator.Name.Value)
		if !found || declaredType == nil || declaredType.Equals(narrowType) {
			continue
		}
		f.track(declarator.Name.Value, narrowType)
	}
}

// observeExpressionStatement is called once per top-level ExpressionStatement
// in a straight-line sequence, wrapping the checker's normal visit of it.
// A bare `name = expr` reassignment keeps narrowing alive (re-tracking with
// the new value, or dropping it if the new value isn't a literal); anything
// else invalidates everything tracked so far.
func (f *flowNarrowState) observeExpressionStatement(c *Checker, node *parser.ExpressionStatement) {
	assign, isAssign := node.Expression.(*parser.AssignmentExpression)
	if !isAssign || assign.Operator != "=" {
		f.invalidateAll()
		c.visit(node.Expression)
		return
	}
	ident, isIdent := assign.Left.(*parser.Identifier)
	if !isIdent {
		f.invalidateAll()
		c.visit(node.Expression)
		return
	}

	// Forget before visiting: checkAssignmentExpression reads the LHS to
	// validate the new value against it, and that must see the declared
	// type, not a narrow type left over from a previous statement.
	f.forget(ident.Value)
	c.visit(node.Expression)

	rhsType := assign.Value.GetComputedType()
	if narrowType, isLiteral := rhsType.(*types.LiteralType); isLiteral {
		f.track(ident.Value, narrowType)
	}
}

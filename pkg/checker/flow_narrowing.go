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

// dropAssignedIn forgets the narrowing of every tracked variable that node
// writes anywhere inside it, before node is visited: reads inside it must not
// see a stale type, and what is true after it is no longer known. Variables
// node leaves alone keep their narrowing across it.
func (f *flowNarrowState) dropAssignedIn(node parser.Node) {
	if len(f.tracked) == 0 {
		return
	}
	writes := map[string]bool{}
	unionSyntacticWrites(writes, node)
	for name := range writes {
		f.forget(name)
	}
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
	if !narrowsOnAssignment(declaredType) {
		return
	}
	f.track(name, narrowType)
}

// narrowsOnAssignment reports whether an assignment narrows a variable of the
// declared type: only unions (and boolean, which is `true | false`) do. A
// `let n = 5` stays a number, so `n === 6` is no error.
func narrowsOnAssignment(declared types.Type) bool {
	switch declared.(type) {
	case *types.UnionType, *types.EnumType:
		return true
	}
	return declared == types.Boolean
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
		if !found || declaredType == nil || declaredType.Equals(narrowType) || !narrowsOnAssignment(declaredType) {
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
	defer c.applyAssertionNarrowing(node.Expression)
	assign, isAssign := node.Expression.(*parser.AssignmentExpression)
	if isAssign && (assign.Operator == "??=" || assign.Operator == "||=") {
		if ident, isIdent := assign.Left.(*parser.Identifier); isIdent {
			f.dropAssignedIn(assign.Value)
			c.visit(node.Expression)
			f.observeLogicalAssignment(c, ident.Value, assign)
			return
		}
	}
	if !isAssign || assign.Operator != "=" {
		f.dropAssignedIn(node)
		c.visit(node.Expression)
		return
	}
	ident, isIdent := assign.Left.(*parser.Identifier)
	if !isIdent {
		f.dropAssignedIn(node)
		c.visit(node.Expression)
		return
	}
	f.dropAssignedIn(assign.Value)

	// Forget before visiting: checkAssignmentExpression reads the LHS to
	// validate the new value against it, and that must see the declared
	// type, not a narrow type left over from a previous statement.
	f.forget(ident.Value)
	c.visit(node.Expression)

	rhsType := assign.Value.GetComputedType()
	if narrowType, isLiteral := rhsType.(*types.LiteralType); isLiteral {
		if narrowsOnAssignment(c.env.ResolveDeclaredType(ident.Value)) {
			f.track(ident.Value, narrowType)
		}
		return
	}
	if rhsType != nil {
		// `x = f()` narrows a declared union to the members f() can be.
		declared := c.env.ResolveDeclaredType(ident.Value)
		if reduced := assignmentReducedType(declared, types.GetWidenedType(rhsType)); reduced != nil {
			f.track(ident.Value, reduced)
		}
	}
}

// observeLogicalAssignment keeps `x ??= v` / `x ||= v` narrowing alive: x is
// then its old non-nullish value or v.
func (f *flowNarrowState) observeLogicalAssignment(c *Checker, name string, assign *parser.AssignmentExpression) {
	before := assign.Left.GetComputedType()
	rhsType := assign.Value.GetComputedType()
	if before == nil || rhsType == nil || rhsType == types.Any || before == types.Any {
		return
	}
	kept := types.RemoveNullishTypes(before)
	if kept == types.Never {
		kept = nil
	}
	joined := types.GetWidenedType(rhsType)
	if kept != nil {
		joined = types.NewUnionType(kept, joined)
	}
	if declared := c.env.ResolveDeclaredType(name); declared != nil && types.IsAssignable(joined, declared) && !c.typesEqual(joined, declared) {
		f.track(name, joined)
	}
}

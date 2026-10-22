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
	// shadowed holds an enclosing sequence's entry this one overwrote (a
	// closure assigning an outer variable), restored when this one drops it.
	shadowed map[string]overlayEntry
}

type overlayEntry struct {
	t   types.Type
	env *Environment
}

func (c *Checker) newFlowNarrowState() *flowNarrowState {
	return &flowNarrowState{c: c, tracked: make(map[string]bool)}
}

// track records that name's flow type is narrowType until something
// invalidates it.
func (f *flowNarrowState) track(name string, narrowType types.Type) {
	if prev, held := f.c.flowNarrowOverlay[name]; held && !f.tracked[name] {
		if f.shadowed == nil {
			f.shadowed = make(map[string]overlayEntry)
		}
		f.shadowed[name] = overlayEntry{prev, f.c.flowNarrowOverlayEnv[name]}
	}
	if f.c.flowNarrowOverlay == nil {
		f.c.flowNarrowOverlay = make(map[string]types.Type)
	}
	f.c.flowNarrowOverlay[name] = narrowType
	if f.c.flowNarrowOverlayEnv == nil {
		f.c.flowNarrowOverlayEnv = make(map[string]*Environment)
	}
	f.c.flowNarrowOverlayEnv[name] = f.c.env
	f.tracked[name] = true
}

// overlayFor returns name's live straight-line narrowing, unless a scope
// opened since it was recorded binds name again (a guard's narrowing inside
// a branch, which is newer).
func (c *Checker) overlayFor(name string) (types.Type, bool) {
	t, ok := c.flowNarrowOverlay[name]
	if !ok || t == nil {
		return nil, false
	}
	stop := c.flowNarrowOverlayEnv[name]
	for e := c.env; e != nil && e != stop; e = e.outer {
		if _, rebound := e.symbols[name]; rebound {
			return nil, false
		}
		// A function body opened since runs at some other time: it reads
		// the declared type, as tsc's captured mutable variables do.
		if e.isFunctionScope {
			return nil, false
		}
	}
	return t, true
}

// forget drops any narrowing this state holds for name, e.g. because it's
// about to be reassigned to something that isn't confidently narrowable, or
// because it's being read as an assignment target (which must see the
// declared type, not a stale narrow one).
func (f *flowNarrowState) forget(name string) {
	if !f.tracked[name] {
		return
	}
	f.release(name)
	delete(f.tracked, name)
}

// release removes name's entry, putting back an enclosing sequence's.
func (f *flowNarrowState) release(name string) {
	if prev, ok := f.shadowed[name]; ok {
		f.c.flowNarrowOverlay[name] = prev.t
		f.c.flowNarrowOverlayEnv[name] = prev.env
		delete(f.shadowed, name)
		return
	}
	delete(f.c.flowNarrowOverlay, name)
}

// invalidateAll drops every narrowing this state holds, because a statement
// follows that this analysis can't linearly account for.
func (f *flowNarrowState) invalidateAll() {
	for name := range f.tracked {
		f.release(name)
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
	unionWritesInScope(writes, node)
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
		if declarator == nil || declarator.Name == nil || declarator.Value == nil {
			continue
		}
		if declarator.TypeAnnotation != nil {
			f.trackAnnotatedInit(c, declarator.Name.Value, declarator.Value.GetComputedType())
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

// trackAnnotatedInit narrows a declared union to what its initializer can
// be, as tsc's assignment narrowing does for the declaration itself
// (`let d: null | Deferred<T> = createDeferred()` is a Deferred<T> after).
func (f *flowNarrowState) trackAnnotatedInit(c *Checker, name string, initType types.Type) {
	if initType == nil || initType == types.Any {
		return
	}
	declared, _, found := c.env.Resolve(name)
	if !found || declared == nil {
		return
	}
	if _, isUnion := declared.(*types.UnionType); !isUnion {
		return
	}
	if reduced := c.assignmentReducedType(declared, c.assignedWidened(initType)); reduced != nil && !reduced.Equals(declared) {
		f.track(name, reduced)
	}
}

// observeExpressionStatement is called once per top-level ExpressionStatement
// in a straight-line sequence, wrapping the checker's normal visit of it.
// A bare `name = expr` reassignment keeps narrowing alive (re-tracking with
// the new value, or dropping it if the new value isn't a literal); anything
// else invalidates everything tracked so far.
func (f *flowNarrowState) observeExpressionStatement(c *Checker, node *parser.ExpressionStatement) {
	c.markUnusedResult(node.Expression)
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

	// The value is read with the narrowing still in place (`x = x.length`
	// after `x = ""`); checkAssignmentExpression validates it against the
	// declared type. The old narrowing ends with the assignment.
	c.visit(node.Expression)
	f.forget(ident.Value)

	rhsType := assign.Value.GetComputedType()
	if narrowType, isLiteral := rhsType.(*types.LiteralType); isLiteral {
		// A value the declared type rejects leaves the declared type.
		if declared := c.env.ResolveDeclaredType(ident.Value); narrowsOnAssignment(declared) && c.rel.IsAssignable(narrowType, declared) {
			f.track(ident.Value, narrowType)
		}
		return
	}
	if rhsType != nil {
		// `x = f()` narrows a declared union to the members f() can be.
		declared := c.env.ResolveDeclaredType(ident.Value)
		if reduced := c.assignmentReducedType(declared, c.assignedWidened(rhsType)); reduced != nil {
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
	joined := c.rel.GetWidenedType(rhsType)
	if kept != nil {
		joined = types.NewUnionType(kept, joined)
	}
	if declared := c.env.ResolveDeclaredType(name); declared != nil && c.rel.IsAssignable(joined, declared) && !c.typesEqual(joined, declared) {
		f.track(name, joined)
	}
}

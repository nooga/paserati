package checker

import (
	"fmt"
	"reflect"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/lexer"

	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// Definite assignment analysis — TS2454, "Variable 'x' is used before being
// assigned."
//
// A forward dataflow walk over each function body (and the top level) that
// tracks which candidate variables are guaranteed to have been written on every
// path reaching a given read. Candidates are `let`/`var` declarations that carry
// a type annotation, have no initializer, no `!` assertion, and whose declared
// type does not admit `undefined`.
//
// The analysis deliberately errs toward silence. Wherever control flow is
// modelled imprecisely (loops, try/catch, switch) it unions in every assignment
// appearing syntactically inside the construct, so an unmodelled path can only
// ever suppress a diagnostic — never invent one. Candidates touched from a
// nested function are dropped entirely, since their flow is unknowable here.

// checkDefiniteAssignmentAssertionInitializer reports TS1263 when a declarator
// combines a `!` definite-assignment assertion with an initializer - the two
// are mutually exclusive (the assertion promises a later, out-of-band write;
// an initializer means there's nothing to promise). This is a syntactic-shape
// rule, not a dataflow one, so unlike checkDefiniteAssignment it is not gated
// by skipDefiniteAssignment and runs for every let/const/var declarator.
func (c *Checker) checkDefiniteAssignmentAssertionInitializer(d *parser.VarDeclarator) {
	if d.DefiniteAssignment && d.Value != nil {
		target := d.Value
		c.addErrorWithCode(target, errors.TS1263, "Declarations with initializers cannot also have definite assignment assertions.")
	}
}

// checkDefiniteAssignment runs the analysis over a whole program.
func (c *Checker) checkDefiniteAssignment(program *parser.Program) {
	if c.skipDefiniteAssignment || program == nil {
		return
	}
	c.analyzeDefiniteAssignmentScope(program.Statements)
}

// collectUnassignedReads runs the definite assignment walk before checking
// and records the reads it may report, with their declarations. tsc types
// such a read with the declared type rather than the narrowed one (the
// TS2454 path of checkIdentifier), so checking needs them up front. Without
// types, every annotated declaration is a candidate and every guard call
// may narrow; readDeclaredTypeIfUnassigned finishes the eligibility test.
func (c *Checker) collectUnassignedReads(program *parser.Program) {
	if c.skipDefiniteAssignment || program == nil {
		return
	}
	c.unassignedReads = map[*parser.Identifier]*parser.VarDeclarator{}
	c.everWritten = map[string]bool{}
	collectAnyWrites(c.everWritten, program)
	c.daCollectOnly = true
	c.analyzeDefiniteAssignmentScope(program.Statements)
	c.daCollectOnly = false
}

// readDeclaredTypeIfUnassigned returns the declared type of a read that may
// see its variable unassigned, when that type does not admit undefined.
func (c *Checker) readDeclaredTypeIfUnassigned(id *parser.Identifier) (types.Type, bool) {
	d := c.unassignedReads[id]
	if d == nil {
		return nil, false
	}
	if d.TypeAnnotation == nil {
		// An auto-typed variable nothing ever writes is undefined at
		// every read in its own flow (tsc's evolving any).
		return types.Undefined, true
	}
	t := declaredVarType(d)
	if t == nil || d.TypeAnnotation == nil || strictInitTypePermits(t) {
		return nil, false
	}
	return t, true
}

// analyzeDefiniteAssignmentScope analyzes one function-level scope: the
// statement list plus every nested function body it contains, each as its own
// independent scope.
func (c *Checker) analyzeDefiniteAssignmentScope(stmts []parser.Statement) {
	a := &definiteAssignment{c: c, candidates: map[string]bool{}, collectOnly: c.daCollectOnly}

	for _, s := range stmts {
		a.collectCandidates(s)
	}

	if len(a.candidates) > 0 {
		assigned := map[string]bool{}
		for _, s := range stmts {
			a.walk(s, assigned)
		}
	}

	// Nested function bodies form their own scopes, analyzed independently.
	for _, s := range stmts {
		c.analyzeNestedFunctionScopes(s)
	}
}

type definiteAssignment struct {
	c *Checker
	// candidates holds names eligible for TS2454. A name is removed when
	// anything makes its flow untrackable (redeclaration, nested-function use).
	candidates map[string]bool
	// reported guards against emitting the same diagnostic twice for a node
	// reached through more than one walk (e.g. a loop body).
	reported map[parser.Node]bool
	// loopHeadVars holds the `var` declarators of for-of/for-in heads.
	loopHeadVars map[*parser.VarDeclarator]bool
	// unreachable is non-zero while walking code a constant condition skips.
	unreachable int
	// collectOnly records reads in c.unassignedReads instead of reporting.
	collectOnly bool
	// decls maps each candidate name to its declarator.
	decls map[string]*parser.VarDeclarator
}

// --- Candidate collection -------------------------------------------------

// collectCandidates records `let`/`var` declarators eligible for TS2454 within
// the current scope, without descending into nested functions. A name seen more
// than once is disqualified: shadowing and redeclaration are tracked by name
// here, so a duplicate makes the state ambiguous.
func (a *definiteAssignment) collectCandidates(node parser.Node) {
	forEachNodeInScope(node, func(n parser.Node) {
		switch s := n.(type) {
		case *parser.ForOfStatement:
			a.noteLoopHeadVars(s.Variable)
		case *parser.ForInStatement:
			a.noteLoopHeadVars(s.Variable)
		case *parser.LetStatement:
			a.collectFromDeclarators(s.Declarations, s.Declare, false)
		case *parser.VarStatement:
			a.collectFromDeclarators(s.Declarations, s.Declare, true)
		}
	})
}

// noteLoopHeadVars records the `var` declarators of a for-of/for-in head. They
// take their type from the iterated value rather than an annotation, but are
// hoisted like any other var, so a read before the loop is a TS2454.
func (a *definiteAssignment) noteLoopHeadVars(variable parser.Statement) {
	vs, ok := variable.(*parser.VarStatement)
	if !ok || vs == nil {
		return
	}
	if a.loopHeadVars == nil {
		a.loopHeadVars = map[*parser.VarDeclarator]bool{}
	}
	for _, d := range vs.Declarations {
		a.loopHeadVars[d] = true
	}
}

func (a *definiteAssignment) collectFromDeclarators(decls []*parser.VarDeclarator, ambient, isVar bool) {
	for _, d := range decls {
		if d == nil || d.Name == nil {
			continue
		}
		name := d.Name.Value
		if _, seen := a.candidates[name]; seen {
			// Redeclared in this scope — stop tracking it entirely.
			a.candidates[name] = false
			continue
		}
		// A `var` that shadows a name the runtime already provides merges with
		// that ambient declaration, which is already initialized.
		if a.c.preexistingGlobals[name] {
			a.candidates[name] = false
			continue
		}
		if a.decls == nil {
			a.decls = map[string]*parser.VarDeclarator{}
		}
		a.decls[name] = d
		if a.collectOnly {
			// Types are not known yet; only annotated declarations can
			// read as their declared type.
			a.candidates[name] = !ambient && d.TypeAnnotation != nil && !d.DefiniteAssignment && (d.Value == nil || isVar)
			// FIXME: tsc's evolving any also types reads after writes (the
			// union of the assigned types); only never-written ones are modelled.
			if !ambient && d.TypeAnnotation == nil && d.Value == nil && !d.DefiniteAssignment &&
				!a.loopHeadVars[d] && a.c.strictNullChecks && !a.c.everWritten[name] {
				a.candidates[name] = true
			}
			continue
		}
		if a.loopHeadVars[d] && d.TypeAnnotation == nil {
			t := declaredVarType(d)
			a.candidates[name] = t != nil && !strictInitTypePermits(t)
			continue
		}
		a.candidates[name] = !ambient && (definiteAssignmentEligible(d) || isVar && initializedVarEligible(d))
	}
}

// definiteAssignmentEligible mirrors TS's requirements for a declaration to
// participate in definite assignment analysis.
func definiteAssignmentEligible(d *parser.VarDeclarator) bool {
	if d.Value != nil || d.DefiniteAssignment || d.TypeAnnotation == nil {
		return false
	}
	// strictInitTypePermits covers `any` and anything admitting `undefined`,
	// which are exactly the types TS lets go uninitialized.
	return !strictInitTypePermits(declaredVarType(d))
}

// initializedVarEligible covers a `var` with an initializer: it is hoisted, so
// a read before the initializer runs (or in a catch clause after the try threw
// first) still sees it unassigned. `let` has TS2448 for that instead.
func initializedVarEligible(d *parser.VarDeclarator) bool {
	if d.Value == nil || d.DefiniteAssignment {
		return false
	}
	t := declaredVarType(d)
	return t != nil && !strictInitTypePermits(t)
}

// declaredVarType returns the resolved type of a declarator. Function-scoped
// declarations get it on the declarator during body checking; top-level ones
// are resolved in Pass 2, which records the type on the name node instead.
func declaredVarType(d *parser.VarDeclarator) types.Type {
	if d.ComputedType != nil {
		return d.ComputedType
	}
	if d.Name != nil {
		return d.Name.GetComputedType()
	}
	return nil
}

// analyzeNestedFunctionScopes recurses into every nested function body found in
// node, analyzing each as an independent scope.
func (c *Checker) analyzeNestedFunctionScopes(node parser.Node) {
	forEachNodeInScope(node, func(n parser.Node) {
		if ns, ok := n.(*parser.NamespaceDeclaration); ok {
			c.analyzeNamespaceScope(ns)
			return
		}
		body, ok := nestedFunctionBody(n)
		if !ok {
			return
		}
		if block, ok := body.(*parser.BlockStatement); ok && block != nil {
			c.analyzeDefiniteAssignmentScope(block.Statements)
		}
	})
}

// analyzeNamespaceScope analyzes a namespace body as its own scope. Inside
// the body an exported variable is read like any other.
func (c *Checker) analyzeNamespaceScope(ns *parser.NamespaceDeclaration) {
	if ns.Declare || ns.Body == nil {
		return
	}
	stmts := make([]parser.Statement, 0, len(ns.Body.Statements))
	for _, s := range ns.Body.Statements {
		if exp, exported := s.(*parser.ExportNamedDeclaration); exported {
			// An exported variable is still a variable inside the body.
			switch d := exp.Declaration.(type) {
			case *parser.VarStatement, *parser.LetStatement, *parser.ConstStatement:
				stmts = append(stmts, d.(parser.Statement))
				continue
			}
			// Still analyze functions and nested namespaces declared there.
			c.analyzeNestedFunctionScopes(s)
			continue
		}
		stmts = append(stmts, s)
	}
	c.analyzeDefiniteAssignmentScope(stmts)
}

// nestedFunctionBody returns the body of a function-like node, and whether node
// was function-like at all. Arrow functions with expression bodies return the
// expression.
func nestedFunctionBody(node parser.Node) (parser.Node, bool) {
	switch n := node.(type) {
	case *parser.FunctionLiteral:
		if n.Body == nil {
			return nil, false
		}
		return n.Body, true
	case *parser.ArrowFunctionLiteral:
		if n.Body == nil {
			return nil, false
		}
		return n.Body, true
	}
	return nil, false
}

// --- Dataflow walk --------------------------------------------------------

// walk processes a statement or expression in execution order, reporting reads
// of unassigned candidates and recording writes into assigned.
func (a *definiteAssignment) walk(node parser.Node, assigned map[string]bool) {
	if isNilNode(node) {
		return
	}

	switch n := node.(type) {

	// --- Declarations ---
	case *parser.LetStatement:
		a.walkDeclarators(n.Declarations, assigned)
	case *parser.VarStatement:
		a.walkDeclarators(n.Declarations, assigned)
	case *parser.ConstStatement:
		a.walkDeclarators(n.Declarations, assigned)

	// --- Writes ---
	case *parser.AssignmentExpression:
		a.walk(n.Value, assigned)
		if id, ok := n.Left.(*parser.Identifier); ok {
			if n.Operator != "=" {
				// Compound assignment reads the target first.
				a.reportIfUnassigned(id, assigned)
			}
			if assignmentDefinitelyWrites(n.Operator) {
				assigned[id.Value] = true
			}
		} else {
			a.walk(n.Left, assigned)
		}
	case *parser.UpdateExpression:
		// TS does not count ++/-- as an assignment for definite assignment:
		// the variable keeps reading as unassigned afterwards.
		if id, ok := n.Argument.(*parser.Identifier); ok {
			a.reportIfUnassigned(id, assigned)
		} else {
			a.walk(n.Argument, assigned)
		}

	case *parser.ArrayDestructuringAssignment:
		a.walk(n.Value, assigned)
		for _, el := range n.Elements {
			a.bindDestructuringElement(el, assigned)
		}
	case *parser.ObjectDestructuringAssignment:
		a.walk(n.Value, assigned)
		for _, prop := range n.Properties {
			if prop == nil {
				continue
			}
			if _, isIdent := prop.Key.(*parser.Identifier); !isIdent {
				a.walk(prop.Key, assigned)
			}
			a.walk(prop.Default, assigned)
			a.markWriteTarget(prop.Target, assigned)
		}
		a.bindDestructuringElement(n.RestProperty, assigned)

	// --- Reads ---
	case *parser.Identifier:
		a.reportIfUnassigned(n, assigned)

	// --- Branching: the one construct modelled precisely ---
	case *parser.IfStatement:
		a.walk(n.Condition, assigned)
		gt, gf := a.conditionGuards(n.Condition)
		cv, cok := constantCondition(n.Condition)
		thenSet := withGuards(assigned, gt)
		a.walkReachable(n.Consequence, thenSet, !cok || cv)
		elseSet := withGuards(assigned, gf)
		a.walkReachable(n.Alternative, elseSet, !cok || !cv)
		mergeBranches(assigned, thenSet, elseSet,
			branchTerminates(n.Consequence) || cok && !cv, branchTerminates(n.Alternative) || cok && cv)
	case *parser.TernaryExpression:
		a.walk(n.Condition, assigned)
		gt, gf := a.conditionGuards(n.Condition)
		cv, cok := constantCondition(n.Condition)
		thenSet := withGuards(assigned, gt)
		a.walkReachable(n.Consequence, thenSet, !cok || cv)
		elseSet := withGuards(assigned, gf)
		a.walkReachable(n.Alternative, elseSet, !cok || !cv)
		mergeBranches(assigned, thenSet, elseSet, cok && !cv, cok && cv)

	// --- Short-circuiting operators: the right side may not run, and sees
	// --- the left side's narrowing ---
	case *parser.InfixExpression:
		a.walk(n.Left, assigned)
		switch n.Operator {
		case "&&", "||":
			gt, gf := a.conditionGuards(n.Left)
			cv, cok := constantCondition(n.Left)
			if n.Operator == "&&" {
				a.walkReachable(n.Right, withGuards(assigned, gt), !cok || cv)
			} else {
				a.walkReachable(n.Right, withGuards(assigned, gf), !cok || !cv)
			}
		case "??":
			a.walk(n.Right, copyAssigned(assigned))
		default:
			a.walk(n.Right, assigned)
		}

	// --- Loops: body runs an unknown number of times ---
	case *parser.WhileStatement:
		a.walk(n.Condition, assigned)
		gt, _ := a.conditionGuards(n.Condition)
		a.walk(n.Body, withGuards(assigned, gt))
		unionSyntacticWrites(assigned, n.Body)
	case *parser.DoWhileStatement:
		// The body always runs at least once, so its writes propagate.
		a.walk(n.Body, assigned)
		a.walk(n.Condition, assigned)
	case *parser.ForStatement:
		a.walk(n.Initializer, assigned)
		a.walk(n.Condition, assigned)
		var gt map[string]bool
		if n.Condition != nil {
			gt, _ = a.conditionGuards(n.Condition)
		}
		body := withGuards(assigned, gt)
		a.walk(n.Body, body)
		a.walk(n.Update, body)
		unionSyntacticWrites(assigned, n.Body)
		unionSyntacticWrites(assigned, n.Update)
	case *parser.ForOfStatement:
		a.walk(n.Iterable, assigned)
		a.bindLoopVariable(n.Variable, assigned)
		a.walkMayNotRun(n.Body, assigned)
	case *parser.ForInStatement:
		a.walk(n.Object, assigned)
		a.bindLoopVariable(n.Variable, assigned)
		a.walkMayNotRun(n.Body, assigned)

	// --- Constructs whose flow is not modelled: walk for reporting, then
	// --- assume every write inside them happened.
	case *parser.TryStatement:
		a.walk(n.Body, copyAssigned(assigned))
		if n.CatchClause != nil {
			a.walk(n.CatchClause.Body, copyAssigned(assigned))
		}
		unionSyntacticWrites(assigned, n.Body)
		if n.CatchClause != nil {
			unionSyntacticWrites(assigned, n.CatchClause.Body)
		}
		// finally always runs, so its writes propagate directly.
		a.walk(n.FinallyBlock, assigned)
	case *parser.SwitchStatement:
		a.walk(n.Expression, assigned)
		// Each case can be reached either by its own match or by falling
		// through from the case immediately before it in source order (JS
		// fallthrough runs cases top-to-bottom once entered, regardless of
		// which one matched). Threading the branch forward - rather than
		// restarting every case from the pre-switch state - models that: a
		// write earlier in the list stays visible to a later case that falls
		// into it. This doesn't try to detect an intervening `break`; that
		// only widens what's considered assigned on fallthrough, which stays
		// within the analysis's existing bias toward silence rather than
		// inventing a diagnostic.
		fallthroughState := assigned
		for _, sc := range n.Cases {
			if sc == nil {
				continue
			}
			branch := copyAssigned(fallthroughState)
			a.walk(sc.Condition, branch)
			a.walk(sc.Body, branch)
			fallthroughState = branch
		}
		for _, sc := range n.Cases {
			if sc == nil {
				continue
			}
			unionSyntacticWrites(assigned, sc.Body)
		}

	// --- Nested functions are separate scopes, skipped here ---
	case *parser.FunctionLiteral, *parser.ArrowFunctionLiteral:
		return

	// --- An immediately invoked function runs inline: TS's flow container
	// --- skips it, so its reads see the enclosing flow.
	case *parser.CallExpression:
		body, isIIFE := nestedFunctionBody(n.Function)
		if !isIIFE {
			forEachChildInScope(node, func(child parser.Node) {
				a.walk(child, assigned)
			})
			return
		}
		for _, arg := range n.Arguments {
			a.walk(arg, assigned)
		}
		if body != nil {
			// Names the function declares itself shadow the outer ones.
			var shadowed []string
			for name := range iifeDeclaredNames(n.Function, body) {
				if a.candidates[name] {
					delete(a.candidates, name)
					shadowed = append(shadowed, name)
				}
			}
			a.walk(body, assigned)
			for _, name := range shadowed {
				a.candidates[name] = true
			}
		}

	// --- Class members and namespace bodies are flow containers of their own
	// --- in TS: reads there see outer variables as initialized and writes
	// --- there do not reach the enclosing flow. Only `extends` and static
	// --- blocks, which run inline during class definition, belong to it.
	case *parser.ClassDeclaration:
		a.walk(n.SuperClass, assigned)
		a.walkStaticBlocks(n.Body, assigned)
	case *parser.ClassExpression:
		a.walk(n.SuperClass, assigned)
		a.walkStaticBlocks(n.Body, assigned)
	case *parser.NamespaceDeclaration:
		return

	// --- Member access: only the object side is a variable read ---
	case *parser.MemberExpression:
		a.walk(n.Object, assigned)
		if _, isIdent := n.Property.(*parser.Identifier); !isIdent {
			a.walk(n.Property, assigned)
		}
	case *parser.OptionalChainingExpression:
		a.walk(n.Object, assigned)

	// --- Object literals: keys are not reads unless computed ---
	case *parser.ObjectLiteral:
		for _, p := range n.Properties {
			if p == nil {
				continue
			}
			if _, isIdent := p.Key.(*parser.Identifier); !isIdent && !isObjectLiteralMethod(p.Value) {
				// A method's or accessor's computed name sits inside the
				// method's own flow container in TS, so it is not checked here.
				a.walk(p.Key, assigned)
			}
			a.walk(p.Value, assigned)
		}

	default:
		// Everything else is a plain container: walk children in order.
		forEachChildInScope(node, func(child parser.Node) {
			a.walk(child, assigned)
		})
	}
}

// assignmentDefinitelyWrites reports whether an assignment operator leaves its
// identifier target assigned. Arithmetic compound assignment does not in TS's
// flow analysis (the result type comes from the unassigned read); `||=` and
// `??=` do, since an unassigned (undefined) target always takes the right side.
func assignmentDefinitelyWrites(op string) bool {
	return op == "=" || op == "||=" || op == "??="
}

func (a *definiteAssignment) walkStaticBlocks(body *parser.ClassBody, assigned map[string]bool) {
	if body == nil {
		return
	}
	for _, b := range body.StaticInitializers {
		a.walk(b, assigned)
	}
}

func isObjectLiteralMethod(v parser.Expression) bool {
	switch f := v.(type) {
	case *parser.ShorthandMethod, *parser.MethodDefinition:
		return true
	case *parser.FunctionLiteral:
		// Methods parse to function literals too; only `key: function () {}`
		// starts with the `function` keyword.
		return f.Token.Type != lexer.FUNCTION
	}
	return false
}

func (a *definiteAssignment) walkDeclarators(decls []*parser.VarDeclarator, assigned map[string]bool) {
	for _, d := range decls {
		if d == nil || d.Name == nil {
			continue
		}
		a.walk(d.Value, assigned)
		if d.Value != nil || !a.candidates[d.Name.Value] {
			// Initialized here, or not something we track — treat as assigned
			// so later reads stay quiet.
			assigned[d.Name.Value] = true
		} else {
			// Declaration without initializer: explicitly unassigned from here.
			delete(assigned, d.Name.Value)
		}
	}
}

// bindLoopVariable handles the binding position of a for-of/for-in head. A
// fresh `let`/`const` declares the variable; a bare identifier or pattern
// assigns to an existing one. Either way it is a write, never a read.
func (a *definiteAssignment) bindLoopVariable(variable parser.Statement, assigned map[string]bool) {
	if isNilNode(variable) {
		return
	}
	if exprStmt, ok := variable.(*parser.ExpressionStatement); ok {
		a.markWriteTarget(exprStmt.Expression, assigned)
		return
	}
	// A declaration in the head is written with each iteration's value.
	markAllIdentifiers(variable, assigned)
}

// bindDestructuringElement records a destructuring target as written, after
// walking any default expression (which is evaluated as a read).
func (a *definiteAssignment) bindDestructuringElement(el *parser.DestructuringElement, assigned map[string]bool) {
	if el == nil {
		return
	}
	a.walk(el.Default, assigned)
	a.markWriteTarget(el.Target, assigned)
}

// markWriteTarget marks an assignment target as written. Nested patterns are
// traversed so every leaf identifier is covered; non-identifier targets (member
// expressions) are walked normally, since their object side is a read.
func (a *definiteAssignment) markWriteTarget(target parser.Expression, assigned map[string]bool) {
	if isNilNode(target) {
		return
	}
	switch t := target.(type) {
	case *parser.Identifier:
		assigned[t.Value] = true
	case *parser.MemberExpression, *parser.IndexExpression,
		*parser.OptionalChainingExpression, *parser.OptionalIndexExpression:
		// `obj.x = v` writes a property; the object itself is read.
		a.walk(target, assigned)
	default:
		// Some other pattern shape — nested arrays, rest elements, parameter
		// patterns. Rather than enumerate every node the parser can produce
		// here, mark every identifier beneath it. Over-marking only silences
		// diagnostics, which is the safe direction for this analysis.
		markAllIdentifiers(target, assigned)
	}
}

// markAllIdentifiers marks every identifier beneath node as assigned.
func markAllIdentifiers(node parser.Node, assigned map[string]bool) {
	forEachDescendant(node, func(n parser.Node) {
		if id, ok := n.(*parser.Identifier); ok {
			assigned[id.Value] = true
		}
	})
}

// walkMayNotRun walks a loop body that may execute zero times: reads inside see
// the pre-loop state, and writes inside do not propagate out — but every write
// appearing inside is unioned in afterward so later reads are not falsely
// flagged.
func (a *definiteAssignment) walkMayNotRun(body parser.Node, assigned map[string]bool) {
	a.walk(body, copyAssigned(assigned))
	unionSyntacticWrites(assigned, body)
}

// walkReachable walks node, reporting nothing in it when it is unreachable.
func (a *definiteAssignment) walkReachable(node parser.Node, assigned map[string]bool, reachable bool) {
	if !reachable {
		a.unreachable++
		defer func() { a.unreachable-- }()
	}
	a.walk(node, assigned)
}

func (a *definiteAssignment) reportIfUnassigned(id *parser.Identifier, assigned map[string]bool) {
	if id == nil || !a.candidates[id.Value] || assigned[id.Value] || a.unreachable > 0 {
		return
	}
	if a.reported == nil {
		a.reported = map[parser.Node]bool{}
	}
	if a.reported[id] {
		return
	}
	a.reported[id] = true
	if a.collectOnly {
		if d := a.decls[id.Value]; d != nil {
			a.c.unassignedReads[id] = d
		}
		return
	}
	a.c.addErrorWithCode(id, errors.TS2454, fmt.Sprintf("Variable '%s' is used before being assigned.", id.Value))
}

// branchTerminates reports whether a conditional arm always exits (return,
// throw, break, continue). A missing arm falls through, so it never terminates.
func branchTerminates(node parser.Node) bool {
	if isNilNode(node) {
		return false
	}
	return blockAlwaysTerminates(node)
}

// --- Set helpers ----------------------------------------------------------

func copyAssigned(src map[string]bool) map[string]bool {
	dst := make(map[string]bool, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// mergeBranches folds the two arms of a conditional back into assigned. A name
// is assigned afterward only if both arms assign it — unless one arm cannot
// fall through, in which case the surviving arm decides on its own.
func mergeBranches(assigned, thenSet, elseSet map[string]bool, thenTerminates, elseTerminates bool) {
	switch {
	case thenTerminates && elseTerminates:
		return
	case thenTerminates:
		adoptAssigned(assigned, elseSet)
	case elseTerminates:
		adoptAssigned(assigned, thenSet)
	default:
		for name := range thenSet {
			if elseSet[name] {
				assigned[name] = true
			}
		}
	}
}

func adoptAssigned(dst, src map[string]bool) {
	for name, ok := range src {
		if ok {
			dst[name] = true
		}
	}
}

// unionSyntacticWrites marks every variable written anywhere inside node as
// assigned, ignoring reachability. Used for constructs whose control flow this
// pass does not model, so an unmodelled path can only silence a diagnostic.
func unionSyntacticWrites(assigned map[string]bool, node parser.Node) {
	forEachDescendant(node, func(n parser.Node) {
		switch w := n.(type) {
		case *parser.AssignmentExpression:
			if id, ok := w.Left.(*parser.Identifier); ok && assignmentDefinitelyWrites(w.Operator) {
				assigned[id.Value] = true
			}
		case *parser.LetStatement:
			markDeclaratorWrites(assigned, w.Declarations)
		case *parser.VarStatement:
			markDeclaratorWrites(assigned, w.Declarations)
		case *parser.ArrayDestructuringAssignment:
			for _, el := range w.Elements {
				markElementWrite(assigned, el)
			}
		case *parser.ObjectDestructuringAssignment:
			for _, prop := range w.Properties {
				if prop != nil {
					markTargetWrite(assigned, prop.Target)
				}
			}
			markElementWrite(assigned, w.RestProperty)
		case *parser.ForOfStatement:
			markLoopVariableWrite(assigned, w.Variable)
		case *parser.ForInStatement:
			markLoopVariableWrite(assigned, w.Variable)
		}
	})
}

// collectAnyWrites records every name anything in node may write: plain,
// compound and logical assignment, ++/--, destructuring and loop heads,
// nested functions included.
func collectAnyWrites(written map[string]bool, node parser.Node) {
	unionSyntacticWrites(written, node)
	forEachDescendant(node, func(n parser.Node) {
		switch w := n.(type) {
		case *parser.AssignmentExpression:
			if id, ok := w.Left.(*parser.Identifier); ok {
				written[id.Value] = true
			}
		case *parser.UpdateExpression:
			if id, ok := w.Argument.(*parser.Identifier); ok {
				written[id.Value] = true
			}
		}
	})
}

func markElementWrite(assigned map[string]bool, el *parser.DestructuringElement) {
	if el != nil {
		markTargetWrite(assigned, el.Target)
	}
}

func markTargetWrite(assigned map[string]bool, target parser.Expression) {
	if isNilNode(target) {
		return
	}
	if id, ok := target.(*parser.Identifier); ok {
		assigned[id.Value] = true
		return
	}
	markAllIdentifiers(target, assigned)
}

func markLoopVariableWrite(assigned map[string]bool, variable parser.Statement) {
	if exprStmt, ok := variable.(*parser.ExpressionStatement); ok {
		markTargetWrite(assigned, exprStmt.Expression)
		return
	}
	if !isNilNode(variable) {
		markAllIdentifiers(variable, assigned)
	}
}

func markDeclaratorWrites(assigned map[string]bool, decls []*parser.VarDeclarator) {
	for _, d := range decls {
		if d != nil && d.Name != nil && d.Value != nil {
			assigned[d.Name.Value] = true
		}
	}
}

// --- Generic traversal ----------------------------------------------------

// isNilNode reports whether node is nil, including a typed-nil interface value.
// AST fields declared as Statement/Expression routinely carry (*T)(nil), which
// is non-nil at the interface level but panics on field access.
func isNilNode(node parser.Node) bool {
	if node == nil {
		return true
	}
	v := reflect.ValueOf(node)
	return v.Kind() == reflect.Ptr && v.IsNil()
}

// forEachChildInScope invokes fn on each immediate child of node, in source
// order. Nested function bodies are passed to fn as-is; it is the caller's job
// to decide whether to descend into them.
func forEachChildInScope(node parser.Node, fn func(parser.Node)) {
	if isNilNode(node) {
		return
	}
	visit := func(n parser.Node) {
		if !isNilNode(n) {
			fn(n)
		}
	}
	visitAll := func(ns []parser.Expression) {
		for _, n := range ns {
			visit(n)
		}
	}

	switch n := node.(type) {
	case *parser.Program:
		for _, s := range n.Statements {
			visit(s)
		}
	case *parser.BlockStatement:
		for _, s := range n.Statements {
			visit(s)
		}
	case *parser.ExpressionStatement:
		visit(n.Expression)
	case *parser.ExportNamedDeclaration:
		// `export function f() { ... }` and `export const x = ...` (#709).
		visit(n.Declaration)
	case *parser.ExportDefaultDeclaration:
		visit(n.Declaration)
	case *parser.ReturnStatement:
		visit(n.ReturnValue)
	case *parser.ThrowStatement:
		visit(n.Value)
	case *parser.LabeledStatement:
		visit(n.Statement)
	case *parser.IfStatement:
		visit(n.Condition)
		visit(n.Consequence)
		visit(n.Alternative)
	case *parser.WhileStatement:
		visit(n.Condition)
		visit(n.Body)
	case *parser.DoWhileStatement:
		visit(n.Body)
		visit(n.Condition)
	case *parser.ForStatement:
		visit(n.Initializer)
		visit(n.Condition)
		visit(n.Update)
		visit(n.Body)
	case *parser.ForOfStatement:
		visit(n.Iterable)
		visit(n.Variable)
		visit(n.Body)
	case *parser.ForInStatement:
		visit(n.Object)
		visit(n.Variable)
		visit(n.Body)
	case *parser.TryStatement:
		visit(n.Body)
		if n.CatchClause != nil {
			visit(n.CatchClause.Body)
		}
		visit(n.FinallyBlock)
	case *parser.SwitchStatement:
		visit(n.Expression)
		for _, sc := range n.Cases {
			if sc == nil {
				continue
			}
			visit(sc.Condition)
			visit(sc.Body)
		}
	case *parser.WithStatement:
		visit(n.Expression)
		visit(n.Body)
	case *parser.LetStatement:
		visitDeclarators(n.Declarations, visit)
	case *parser.VarStatement:
		visitDeclarators(n.Declarations, visit)
	case *parser.ConstStatement:
		visitDeclarators(n.Declarations, visit)
	case *parser.AssignmentExpression:
		visit(n.Left)
		visit(n.Value)
	case *parser.UpdateExpression:
		visit(n.Argument)
	case *parser.InfixExpression:
		visit(n.Left)
		visit(n.Right)
	case *parser.PrefixExpression:
		visit(n.Right)
	case *parser.TernaryExpression:
		visit(n.Condition)
		visit(n.Consequence)
		visit(n.Alternative)
	case *parser.CallExpression:
		visit(n.Function)
		visitAll(n.Arguments)
	case *parser.NewExpression:
		visit(n.Constructor)
		visitAll(n.Arguments)
	case *parser.OptionalCallExpression:
		visit(n.Function)
		visitAll(n.Arguments)
	case *parser.MemberExpression:
		visit(n.Object)
		visit(n.Property)
	case *parser.OptionalChainingExpression:
		visit(n.Object)
	case *parser.IndexExpression:
		visit(n.Left)
		visit(n.Index)
	case *parser.OptionalIndexExpression:
		visit(n.Object)
		visit(n.Index)
	case *parser.ArrayLiteral:
		visitAll(n.Elements)
	case *parser.ObjectLiteral:
		for _, p := range n.Properties {
			if p == nil {
				continue
			}
			visit(p.Key)
			visit(p.Value)
		}
	case *parser.SpreadElement:
		visit(n.Argument)
	case *parser.ComputedPropertyName:
		visit(n.Expr)
	case *parser.ArrayDestructuringAssignment:
		visit(n.Value)
		for _, el := range n.Elements {
			if el != nil {
				visit(el.Target)
				visit(el.Default)
			}
		}
	case *parser.ObjectDestructuringAssignment:
		visit(n.Value)
		for _, prop := range n.Properties {
			if prop != nil {
				visit(prop.Target)
				visit(prop.Default)
			}
		}
		if n.RestProperty != nil {
			visit(n.RestProperty.Target)
		}
	case *parser.TemplateLiteral:
		for _, part := range n.Parts {
			visit(part)
		}
	case *parser.TaggedTemplateExpression:
		visit(n.Tag)
		visit(n.Template)
	case *parser.AwaitExpression:
		visit(n.Argument)
	case *parser.YieldExpression:
		visit(n.Value)
	case *parser.TypeofExpression:
		visit(n.Operand)
	case *parser.TypeAssertionExpression:
		visit(n.Expression)
	case *parser.SatisfiesExpression:
		visit(n.Expression)
	case *parser.NonNullExpression:
		visit(n.Expression)
	case *parser.FunctionLiteral:
		visit(n.Body)
	case *parser.ArrowFunctionLiteral:
		visit(n.Body)
	case *parser.ClassDeclaration:
		visit(n.SuperClass)
		visitClassBody(n.Body, visit)
	case *parser.ClassExpression:
		visit(n.SuperClass)
		visitClassBody(n.Body, visit)
	case *parser.MethodDefinition:
		visit(n.Value)
	case *parser.PropertyDefinition:
		visit(n.Value)
	case *parser.ShorthandMethod:
		visit(n.Body)
	}
}

// visitClassBody yields the parts of a class body that can contain code:
// method implementations, property initializers and static blocks.
func visitClassBody(body *parser.ClassBody, visit func(parser.Node)) {
	if body == nil {
		return
	}
	for _, m := range body.Methods {
		if m != nil {
			visit(m)
		}
	}
	for _, p := range body.Properties {
		if p != nil {
			visit(p)
		}
	}
	for _, b := range body.StaticInitializers {
		visit(b)
	}
}

func visitDeclarators(decls []*parser.VarDeclarator, visit func(parser.Node)) {
	for _, d := range decls {
		if d == nil {
			continue
		}
		if d.Name != nil {
			visit(d.Name)
		}
		visit(d.Value)
	}
}

// forEachNodeInScope invokes fn on node and every descendant belonging to the
// same function scope. Function-like nodes are passed to fn but not descended
// into — their bodies are separate scopes, analyzed on their own.
func forEachNodeInScope(node parser.Node, fn func(parser.Node)) {
	if isNilNode(node) {
		return
	}
	fn(node)
	if _, isFunc := nestedFunctionBody(node); isFunc {
		return
	}
	forEachChildInScope(node, func(child parser.Node) {
		forEachNodeInScope(child, fn)
	})
}

// forEachDescendant invokes fn on node and every node beneath it, including
// nested function bodies.
func forEachDescendant(node parser.Node, fn func(parser.Node)) {
	if isNilNode(node) {
		return
	}
	fn(node)
	forEachChildInScope(node, func(child parser.Node) {
		forEachDescendant(child, fn)
	})
}

// iifeDeclaredNames lists the names a function literal declares: its
// parameters and the variables, functions and classes of its body.
func iifeDeclaredNames(fn parser.Expression, body parser.Node) map[string]bool {
	names := map[string]bool{}
	var params []*parser.Parameter
	switch f := fn.(type) {
	case *parser.FunctionLiteral:
		params = f.Parameters
		if f.RestParameter != nil && f.RestParameter.Name != nil {
			names[f.RestParameter.Name.Value] = true
		}
	case *parser.ArrowFunctionLiteral:
		params = f.Parameters
		if f.RestParameter != nil && f.RestParameter.Name != nil {
			names[f.RestParameter.Name.Value] = true
		}
	}
	for _, p := range params {
		if p == nil {
			continue
		}
		if p.Name != nil {
			names[p.Name.Value] = true
		}
		if p.Pattern != nil {
			forEachNodeInScope(p.Pattern, func(n parser.Node) {
				if id, ok := n.(*parser.Identifier); ok {
					names[id.Value] = true
				}
			})
		}
	}
	if block, ok := body.(*parser.BlockStatement); ok {
		forEachNodeInScope(block, func(n parser.Node) {
			switch d := n.(type) {
			case *parser.VarStatement:
				for _, decl := range d.Declarations {
					if decl != nil && decl.Name != nil {
						names[decl.Name.Value] = true
					}
				}
			case *parser.LetStatement:
				for _, decl := range d.Declarations {
					if decl != nil && decl.Name != nil {
						names[decl.Name.Value] = true
					}
				}
			case *parser.ConstStatement:
				for _, decl := range d.Declarations {
					if decl != nil && decl.Name != nil {
						names[decl.Name.Value] = true
					}
				}
			case *parser.ClassDeclaration:
				if d.Name != nil {
					names[d.Name.Value] = true
				}
			case *parser.ExpressionStatement:
				if fl, isFn := d.Expression.(*parser.FunctionLiteral); isFn && fl.Name != nil {
					names[fl.Name.Value] = true
				}
			}
		})
	}
	return names
}

package checker

import (
	"fmt"
	"reflect"

	"github.com/nooga/paserati/pkg/errors"

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

// checkDefiniteAssignment runs the analysis over a whole program.
func (c *Checker) checkDefiniteAssignment(program *parser.Program) {
	if c.skipDefiniteAssignment || program == nil {
		return
	}
	c.analyzeDefiniteAssignmentScope(program.Statements)
}

// analyzeDefiniteAssignmentScope analyzes one function-level scope: the
// statement list plus every nested function body it contains, each as its own
// independent scope.
func (c *Checker) analyzeDefiniteAssignmentScope(stmts []parser.Statement) {
	a := &definiteAssignment{c: c, candidates: map[string]bool{}}

	for _, s := range stmts {
		a.collectCandidates(s)
	}
	for _, s := range stmts {
		a.disqualifyFromNestedFunctions(s)
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
}

// --- Candidate collection -------------------------------------------------

// collectCandidates records `let`/`var` declarators eligible for TS2454 within
// the current scope, without descending into nested functions. A name seen more
// than once is disqualified: shadowing and redeclaration are tracked by name
// here, so a duplicate makes the state ambiguous.
func (a *definiteAssignment) collectCandidates(node parser.Node) {
	forEachNodeInScope(node, func(n parser.Node) {
		switch s := n.(type) {
		case *parser.LetStatement:
			a.collectFromDeclarators(s.Declarations, s.Declare)
		case *parser.VarStatement:
			a.collectFromDeclarators(s.Declarations, s.Declare)
		}
	})
}

func (a *definiteAssignment) collectFromDeclarators(decls []*parser.VarDeclarator, ambient bool) {
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
		a.candidates[name] = !ambient && definiteAssignmentEligible(d)
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

// disqualifyFromNestedFunctions drops any candidate that a nested function
// reads or writes. TS models those flows through closures; this pass does not,
// so it declines to report on them rather than risk a false positive.
func (a *definiteAssignment) disqualifyFromNestedFunctions(node parser.Node) {
	forEachNodeInScope(node, func(n parser.Node) {
		if body, ok := nestedFunctionBody(n); ok {
			forEachDescendant(body, func(d parser.Node) {
				if id, ok := d.(*parser.Identifier); ok {
					delete(a.candidates, id.Value)
				}
			})
		}
	})
}

// analyzeNestedFunctionScopes recurses into every nested function body found in
// node, analyzing each as an independent scope.
func (c *Checker) analyzeNestedFunctionScopes(node parser.Node) {
	forEachNodeInScope(node, func(n parser.Node) {
		body, ok := nestedFunctionBody(n)
		if !ok {
			return
		}
		if block, ok := body.(*parser.BlockStatement); ok && block != nil {
			c.analyzeDefiniteAssignmentScope(block.Statements)
		}
	})
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
			assigned[id.Value] = true
		} else {
			a.walk(n.Left, assigned)
		}
	case *parser.UpdateExpression:
		if id, ok := n.Argument.(*parser.Identifier); ok {
			a.reportIfUnassigned(id, assigned)
			assigned[id.Value] = true
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
		thenSet := copyAssigned(assigned)
		a.walk(n.Consequence, thenSet)
		elseSet := copyAssigned(assigned)
		a.walk(n.Alternative, elseSet)
		mergeBranches(assigned, thenSet, elseSet,
			branchTerminates(n.Consequence), branchTerminates(n.Alternative))
	case *parser.TernaryExpression:
		a.walk(n.Condition, assigned)
		thenSet := copyAssigned(assigned)
		a.walk(n.Consequence, thenSet)
		elseSet := copyAssigned(assigned)
		a.walk(n.Alternative, elseSet)
		mergeBranches(assigned, thenSet, elseSet, false, false)

	// --- Short-circuiting operators: the right side may not run ---
	case *parser.InfixExpression:
		a.walk(n.Left, assigned)
		if n.Operator == "&&" || n.Operator == "||" || n.Operator == "??" {
			a.walk(n.Right, copyAssigned(assigned))
		} else {
			a.walk(n.Right, assigned)
		}

	// --- Loops: body runs an unknown number of times ---
	case *parser.WhileStatement:
		a.walk(n.Condition, assigned)
		a.walkMayNotRun(n.Body, assigned)
	case *parser.DoWhileStatement:
		// The body always runs at least once, so its writes propagate.
		a.walk(n.Body, assigned)
		a.walk(n.Condition, assigned)
	case *parser.ForStatement:
		a.walk(n.Initializer, assigned)
		a.walk(n.Condition, assigned)
		body := copyAssigned(assigned)
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
		for _, sc := range n.Cases {
			if sc == nil {
				continue
			}
			branch := copyAssigned(assigned)
			a.walk(sc.Condition, branch)
			a.walk(sc.Body, branch)
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

	// --- Class and namespace bodies run code this pass does not order (static
	// --- blocks, field initializers), so assume every write inside happened.
	case *parser.ClassDeclaration, *parser.ClassExpression, *parser.NamespaceDeclaration:
		unionSyntacticWrites(assigned, node)

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
			if _, isIdent := p.Key.(*parser.Identifier); !isIdent {
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
	a.walk(variable, assigned)
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

func (a *definiteAssignment) reportIfUnassigned(id *parser.Identifier, assigned map[string]bool) {
	if id == nil || !a.candidates[id.Value] || assigned[id.Value] {
		return
	}
	if a.reported == nil {
		a.reported = map[parser.Node]bool{}
	}
	if a.reported[id] {
		return
	}
	a.reported[id] = true
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
			if id, ok := w.Left.(*parser.Identifier); ok {
				assigned[id.Value] = true
			}
		case *parser.UpdateExpression:
			if id, ok := w.Argument.(*parser.Identifier); ok {
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

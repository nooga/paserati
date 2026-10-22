package checker

import (
	"sort"
	"strings"

	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// This file holds the join logic for assignment narrowing at branch points.
//
// A branch (the arms of an if, the cases of a switch, a loop body, a try
// block) is checked in its own scope, seeded with the current type of every
// reference it writes. Assignments inside the branch then update that scope
// instead of a narrowing that belongs to the code around the branch, and the
// types the branch ends with can be joined where control flow merges again.

// writtenRefs lists, sorted, the narrowing keys written anywhere inside the
// nodes: plain variables (`x = ...`) and member chains (`this.a.b = ...`).
func writtenRefs(nodes ...parser.Node) []string {
	set := map[string]bool{}
	for _, node := range nodes {
		if isNilNode(node) {
			continue
		}
		unionSyntacticWrites(set, node)
		forEachDescendant(node, func(n parser.Node) {
			if assign, ok := n.(*parser.AssignmentExpression); ok {
				if member, ok := assign.Left.(*parser.MemberExpression); ok {
					if key := expressionToNarrowingKey(member); key != "" {
						set[key] = true
					}
				}
			}
		})
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// typeOfRefIn is the type env gives a narrowing key: a variable's type, or
// the nearest narrowing of a member chain. It is nil when nothing is known.
func typeOfRefIn(env *Environment, key string) types.Type {
	if env == nil {
		return nil
	}
	if !strings.Contains(key, ".") {
		t, _, found := env.Resolve(key)
		if !found {
			return nil
		}
		return t
	}
	for e := env; e != nil; e = e.outer {
		if t, ok := e.narrowings[key]; ok {
			return t
		}
	}
	return nil
}

// seedBranchEnv returns base (or, when base is nil, a new scope inside the
// current one) holding the current type of every key, so a branch that writes
// them updates this scope and leaves the surrounding narrowing alone.
func (c *Checker) seedBranchEnv(base *Environment, keys []string) *Environment {
	return c.seedBranchEnvWith(base, keys, false)
}

// declaredTypeOfRef is the type a reference was declared with, ignoring any
// narrowing: nil for a name or member the checker cannot resolve.
func (c *Checker) declaredTypeOfRef(key string) types.Type {
	if strings.Contains(key, ".") {
		return c.resolveMemberExpressionOriginalType(key)
	}
	if t := c.env.ResolveDeclaredType(key); t != nil {
		return t
	}
	t, _, _ := c.env.Resolve(key)
	return t
}

// seedBranchEnvWith is seedBranchEnv, optionally seeding the declared type
// rather than the current one (for a loop body, which can run after any
// assignment in it).
func (c *Checker) seedBranchEnvWith(base *Environment, keys []string, declared bool) *Environment {
	env := base
	lookup := func() *Environment {
		if env != nil {
			return env
		}
		return c.env
	}
	for _, key := range keys {
		if strings.Contains(key, ".") {
			t := typeOfRefIn(lookup(), key)
			if t == nil && typeOfRefIn(lookup(), key+"__complement") != nil {
				// Narrowed by subtraction (`!== undefined`); seeding the
				// declared type would override that.
				continue
			}
			if declared || t == nil {
				if d := c.resolveMemberExpressionOriginalType(key); d != nil {
					t = d
				}
			}
			if t == nil {
				continue
			}
			if env == nil {
				env = NewEnclosedEnvironment(c.env)
			}
			if _, ok := env.narrowings[key]; !ok {
				env.narrowings[key] = t
			}
			continue
		}
		t, isConst, found := lookup().Resolve(key)
		if !found || isConst || t == nil {
			continue
		}
		if declared {
			if d := c.env.ResolveDeclaredType(key); d != nil {
				t = d
			}
		} else {
			// A straight-line narrowing newer than the binding found
			// (`let x: string | number = 42`) is what the arm starts with;
			// rebinding the declared type would hide it.
			saved := c.env
			c.env = lookup()
			if ot, live := c.overlayFor(key); live {
				t = ot
			}
			c.env = saved
		}
		if env == nil {
			env = NewEnclosedEnvironment(c.env)
		}
		if _, ok := env.symbols[key]; !ok {
			env.Define(key, t, false)
		}
	}
	return env
}

// branchEnd is where one arm of a branch ended: the scope holding what it
// narrowed or assigned, and whether control can leave it other than by
// falling out of the end.
type branchEnd struct {
	env        *Environment
	terminates bool
}

// joinBranches makes the code after a branch see, for each key, the union of
// what the arms that fall through ended with. It leaves c.env alone when
// nothing changed or no arm falls through.
func (c *Checker) joinBranches(originalEnv *Environment, keys []string, arms []branchEnd) {
	var live []*Environment
	for _, arm := range arms {
		if !arm.terminates {
			live = append(live, arm.env)
		}
	}
	if len(live) == 0 {
		return
	}
	merged := map[string]types.Type{}
	for _, key := range keys {
		var joined types.Type
		for _, env := range live {
			t := typeOfRefIn(env, key)
			if t == nil {
				joined = nil
				break
			}
			if joined == nil {
				joined = t
			} else {
				joined = c.computeMergedType(joined, t)
			}
		}
		if joined == nil {
			continue
		}
		if cur := typeOfRefIn(originalEnv, key); cur != nil && c.typesEqual(joined, cur) {
			continue
		}
		merged[key] = joined
	}
	// A reference an enclosing branch already holds a scope entry for is
	// updated there, so the join is still seen when that branch ends.
	var mergedEnv *Environment
	for key, t := range merged {
		if strings.Contains(key, ".") {
			if c.hasNarrowingInChain(key) {
				c.updateNarrowingInChain(key, t)
				continue
			}
		} else if originalEnv.UpdateInChain(key, t) {
			continue
		}
		if mergedEnv == nil {
			mergedEnv = NewEnclosedEnvironment(originalEnv)
		}
		if strings.Contains(key, ".") {
			mergedEnv.narrowings[key] = t
			continue
		}
		_, isConst, _ := originalEnv.Resolve(key)
		mergedEnv.Define(key, t, isConst)
	}
	if mergedEnv != nil {
		c.env = mergedEnv
	}
}

// conditionRefKeys are the references an if condition tests: the one a type
// guard narrows, `!ref`, and a bare `ref`.
func conditionRefKeys(condition parser.Expression, guard *TypeGuard) []string {
	var keys []string
	if guard != nil && guard.VariableName != "" {
		keys = append(keys, guard.VariableName)
	}
	if key := falsyGuardKey(condition); key != "" {
		keys = append(keys, key)
	}
	switch condition.(type) {
	case *parser.Identifier, *parser.MemberExpression:
		if key := expressionToNarrowingKey(condition); key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

func appendUnique(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}

// completeMemberGuardElse gives the else side of a type guard on a member
// chain (`this.items === null`) the declared type minus the guarded type,
// where inverting the guard did not produce one.
func (c *Checker) completeMemberGuardElse(elseEnv *Environment, guard *TypeGuard) *Environment {
	if guard == nil || !strings.Contains(guard.VariableName, ".") || guard.NarrowedType == nil || isNarrowingMarker(guard.NarrowedType) {
		return elseEnv
	}
	if typeOfRefIn(elseEnv, guard.VariableName) != nil {
		return elseEnv
	}
	orig := c.resolveMemberExpressionOriginalType(guard.VariableName)
	if orig == nil {
		return elseEnv
	}
	var t types.Type
	if guard.IsNegated {
		t = guard.NarrowedType
	} else {
		t = c.subtractTypeFromUnion(orig, guard.NarrowedType)
	}
	if t == nil || t == types.Never {
		return elseEnv
	}
	if elseEnv == nil {
		elseEnv = NewEnclosedEnvironment(c.env)
	}
	elseEnv.narrowings[guard.VariableName] = t
	return elseEnv
}

// checkLoop checks a loop with the references it writes seeded at their
// declared types, so nothing narrowed before the loop is assumed inside it and
// nothing assigned in it leaks out. Afterwards those references have their
// declared types again, narrowed by the loop condition being false when the
// loop cannot be left any other way.
func (c *Checker) checkLoop(loop parser.Node, cond parser.Expression, check func()) {
	originalEnv := c.env
	keys := writtenRefs(loop)
	if len(keys) == 0 {
		check()
		return
	}
	seeds := c.loopFixpointSeeds(loop, keys, originalEnv, check)
	c.loopEntryTypes = nil
	enter := func() {
		env := NewEnclosedEnvironment(originalEnv)
		for key, t := range seeds {
			_, isConst, _ := originalEnv.Resolve(key)
			env.Define(key, t, isConst)
		}
		if seeded := c.seedBranchEnvWith(env, keys, true); seeded != nil {
			env = seeded
		}
		c.env = env
	}
	enter()
	check()
	c.env = originalEnv

	post := NewEnclosedEnvironment(originalEnv)
	changed := false
	for _, key := range keys {
		declared := c.declaredTypeOfRef(key)
		if seed, ok := seeds[key]; ok {
			// After the loop: what it was entered with or assigned.
			declared = seed
		}
		current := typeOfRefIn(originalEnv, key)
		if declared == nil || current == nil || c.typesEqual(declared, current) {
			continue
		}
		changed = true
		if strings.Contains(key, ".") {
			post.narrowings[key] = declared
		} else {
			_, isConst, _ := originalEnv.Resolve(key)
			post.Define(key, declared, isConst)
		}
	}
	if changed {
		c.env = post
	}
	if cond == nil || loopHasBreak(loop) {
		return
	}
	var exit *Environment
	if guard := c.detectTypeGuard(cond); guard != nil {
		exit = c.applyInvertedTypeNarrowing(guard)
		exit = c.completeMemberGuardElse(exit, guard)
	}
	if exit == nil {
		exit = c.applyInvertedTruthinessNarrowing(cond)
	}
	if exit != nil {
		c.env = exit
	}
}

// loopHasBreak reports whether a break appears anywhere inside the loop.
func loopHasBreak(loop parser.Node) bool {
	found := false
	forEachDescendant(loop, func(n parser.Node) {
		if _, ok := n.(*parser.BreakStatement); ok {
			found = true
		}
	})
	return found
}

// loopHasContinue reports whether a continue appears anywhere inside n.
func loopHasContinue(n parser.Node) bool {
	found := false
	forEachDescendant(n, func(n parser.Node) {
		if _, ok := n.(*parser.ContinueStatement); ok {
			found = true
		}
	})
	return found
}

// resetToDeclared makes the code after a construct we do not model see the
// declared type of every reference it wrote.
func (c *Checker) resetToDeclared(originalEnv *Environment, keys []string) {
	post := NewEnclosedEnvironment(originalEnv)
	changed := false
	for _, key := range keys {
		declared := c.declaredTypeOfRef(key)
		current := typeOfRefIn(originalEnv, key)
		if declared == nil || current == nil || c.typesEqual(declared, current) {
			continue
		}
		changed = true
		if strings.Contains(key, ".") {
			post.narrowings[key] = declared
		} else {
			_, isConst, _ := originalEnv.Resolve(key)
			post.Define(key, declared, isConst)
		}
	}
	if changed {
		c.env = post
	}
}

// switchBodies are the case bodies of a switch, as nodes.
func switchBodies(node *parser.SwitchStatement) []parser.Node {
	bodies := make([]parser.Node, 0, len(node.Cases))
	for _, cc := range node.Cases {
		if cc.Body != nil {
			bodies = append(bodies, cc.Body)
		}
	}
	return bodies
}

// hasBreak reports whether a break statement occurs anywhere inside node.
func hasBreak(node parser.Node) bool {
	return breaksOutOfSwitch(node)
}

// breaksOnlyAtEnd reports whether a case body leaves the switch by a break
// that is its last statement and contains no other break.
func breaksOnlyAtEnd(body *parser.BlockStatement) bool {
	if body == nil || len(body.Statements) == 0 {
		return false
	}
	n := len(body.Statements)
	if _, ok := body.Statements[n-1].(*parser.BreakStatement); !ok {
		return false
	}
	for _, st := range body.Statements[:n-1] {
		if breaksOutOfSwitch(st) {
			return false
		}
	}
	return true
}

// switchCoversAll reports whether the cases cover every member of the
// switched expression's type, a union of literal types.
func (c *Checker) switchCoversAll(node *parser.SwitchStatement, exprType types.Type) bool {
	members := []types.Type{exprType}
	if u, ok := exprType.(*types.UnionType); ok {
		members = u.Types
	}
	for _, m := range members {
		switch m.(type) {
		case *types.LiteralType, *types.EnumMemberType:
		default:
			if m != types.Null && m != types.Undefined {
				return false
			}
		}
		covered := false
		for _, cc := range node.Cases {
			if cc.Condition == nil {
				continue
			}
			if t := cc.Condition.GetComputedType(); t != nil && t.Equals(m) {
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

// visitNarrowedBy checks a loop body in the environment its condition being
// true gives.
func (c *Checker) visitNarrowedBy(cond parser.Expression, body parser.Node) {
	if isNilNode(cond) {
		c.visit(body)
		return
	}
	outer := c.env
	narrowed := c.applyTypeNarrowingWithFallback(cond)
	if narrowed != nil {
		c.env = narrowed
	}
	c.visit(body)
	c.env = outer
}

// earlyExitRefKeys lists the references the statements directly inside the
// arms narrow by leaving early: `if (cond) return;` narrows what cond tests
// for the rest of the arm.
func earlyExitRefKeys(c *Checker, arms ...parser.Statement) []string {
	var keys []string
	for _, arm := range arms {
		block, ok := arm.(*parser.BlockStatement)
		if !ok || block == nil {
			continue
		}
		for _, st := range block.Statements {
			// An assertion call narrows the rest of the arm as an early
			// exit would (`if (x !== undefined) { assertIsString(x); }`
			// leaves x string | undefined).
			if key := c.assertedRefKey(st); key != "" {
				keys = appendUnique(keys, key)
				continue
			}
			ifs, ok := st.(*parser.IfStatement)
			if !ok || ifs == nil || !isNilNode(ifs.Alternative) || !blockAlwaysTerminates(ifs.Consequence) {
				continue
			}
			for _, key := range conditionRefKeys(ifs.Condition, c.detectTypeGuard(ifs.Condition)) {
				// FIXME: member chains are left out; seeding them disturbs
				// sibling member narrowings in the arm.
				if !strings.Contains(key, ".") {
					keys = appendUnique(keys, key)
				}
			}
		}
	}
	return keys
}

// loopFixpointSeeds computes, for the plain variables of a declared union type
// that the loop writes only by `x = ...` in its own code, the type tsc's loop
// flow analysis gives them inside the loop: the type the loop is entered
// with, joined with every type assigned in it, iterated (speculatively, a few
// times) until it is stable. Other written references keep their declared
// types.
func (c *Checker) loopFixpointSeeds(loop parser.Node, keys []string, originalEnv *Environment, check func()) map[string]types.Type {
	assigns := map[string][]*parser.AssignmentExpression{}
	seeds := map[string]types.Type{}
	for _, key := range keys {
		if strings.Contains(key, ".") {
			continue
		}
		declared := c.declaredTypeOfRef(key)
		if _, isUnion := declared.(*types.UnionType); !isUnion {
			continue
		}
		entry := typeOfRefIn(originalEnv, key)
		if t, ok := c.loopEntryTypes[key]; ok {
			// A straight-line narrowing the loop's writes dropped, as the
			// declared members it narrowed to.
			entry = t
			if reduced := c.assignmentReducedType(declared, c.assignedWidened(t)); reduced != nil {
				entry = reduced
			}
		}
		if entry == nil || c.typesEqual(entry, declared) {
			continue
		}
		list, ok := simpleLoopAssignments(loop, key)
		if !ok {
			continue
		}
		assigns[key] = list
		seeds[key] = entry
	}
	if len(seeds) == 0 {
		return nil
	}
	for iter := 0; iter < 3; iter++ {
		c.speculate(func() {
			env := NewEnclosedEnvironment(originalEnv)
			for key, t := range seeds {
				_, isConst, _ := originalEnv.Resolve(key)
				env.Define(key, t, isConst)
			}
			if seeded := c.seedBranchEnvWith(env, keys, true); seeded != nil {
				env = seeded
			}
			c.env = env
			check()
			c.env = originalEnv
		})
		stable := true
		for key, list := range assigns {
			declared := c.declaredTypeOfRef(key)
			next := seeds[key]
			for _, a := range list {
				rhs := a.Value.GetComputedType()
				if rhs == nil {
					next = declared
					break
				}
				assigned := c.assignmentReducedType(declared, c.assignedWidened(rhs))
				if assigned == nil {
					assigned = declared
				}
				next = c.computeMergedType(next, assigned)
			}
			if !c.typesEqual(next, seeds[key]) {
				seeds[key] = next
				stable = false
			}
		}
		if stable {
			break
		}
	}
	return seeds
}

// simpleLoopAssignments lists the `key = ...` assignments in loop, or reports
// false when key is written any other way (compound or destructuring
// assignment, ++, a declaration, a loop head) or from a nested function.
func simpleLoopAssignments(loop parser.Node, key string) ([]*parser.AssignmentExpression, bool) {
	var list []*parser.AssignmentExpression
	ok := true
	var walk func(n parser.Node, nested bool)
	walk = func(n parser.Node, nested bool) {
		if !ok || isNilNode(n) {
			return
		}
		switch w := n.(type) {
		case *parser.FunctionLiteral, *parser.ArrowFunctionLiteral, *parser.ShorthandMethod,
			*parser.ClassExpression, *parser.ClassDeclaration:
			nested = true
		case *parser.AssignmentExpression:
			if id, isIdent := w.Left.(*parser.Identifier); isIdent && id.Value == key {
				if nested || w.Operator != "=" {
					ok = false
					return
				}
				list = append(list, w)
			}
		case *parser.UpdateExpression:
			if id, isIdent := w.Argument.(*parser.Identifier); isIdent && id.Value == key {
				ok = false
				return
			}
		}
		forEachChildInScope(n, func(child parser.Node) { walk(child, nested) })
	}
	walk(loop, false)
	if !ok {
		return nil, false
	}
	// Any write the walk did not see (a declaration, destructuring, a loop
	// head) makes the variable's flow unknown here.
	others := map[string]bool{}
	unionSyntacticWrites(others, loop)
	if others[key] && len(list) == 0 {
		return nil, false
	}
	return list, writesOnlyByAssignment(loop, key)
}

// writesOnlyByAssignment reports whether every write of key in node is a
// plain assignment expression.
func writesOnlyByAssignment(node parser.Node, key string) bool {
	ok := true
	forEachDescendant(node, func(n parser.Node) {
		switch w := n.(type) {
		case *parser.LetStatement:
			for _, d := range w.Declarations {
				if d != nil && d.Name != nil && d.Name.Value == key {
					ok = false
				}
			}
		case *parser.VarStatement:
			for _, d := range w.Declarations {
				if d != nil && d.Name != nil && d.Name.Value == key {
					ok = false
				}
			}
		case *parser.ArrayDestructuringAssignment, *parser.ObjectDestructuringAssignment, *parser.ForOfStatement, *parser.ForInStatement:
			set := map[string]bool{}
			unionSyntacticWrites(set, n)
			if set[key] {
				ok = false
			}
		}
	})
	return ok
}

// noteLoopEntryTypes records, before a loop statement is checked, the
// straight-line narrowings of the variables it writes.
func (c *Checker) noteLoopEntryTypes(stmt parser.Statement) {
	switch stmt.(type) {
	case *parser.WhileStatement, *parser.DoWhileStatement, *parser.ForStatement,
		*parser.ForOfStatement, *parser.ForInStatement:
	default:
		return
	}
	c.loopEntryTypes = nil
	for _, key := range writtenRefs(stmt) {
		if t, ok := c.flowNarrowOverlay[key]; ok {
			if c.loopEntryTypes == nil {
				c.loopEntryTypes = map[string]types.Type{}
			}
			c.loopEntryTypes[key] = t
		}
	}
}

// assertedRefKey is the plain variable an assertion-function call statement
// st narrows (`assertIsString(x);`), or "".
func (c *Checker) assertedRefKey(st parser.Statement) string {
	es, ok := st.(*parser.ExpressionStatement)
	if !ok || es == nil {
		return ""
	}
	call, ok := es.Expression.(*parser.CallExpression)
	if !ok {
		return ""
	}
	ident, ok := call.Function.(*parser.Identifier)
	if !ok {
		return ""
	}
	fnType, _, found := c.env.Resolve(ident.Value)
	if !found {
		return ""
	}
	if pred := extractTypePredicateFromType(fnType); pred == nil || pred.predicate == nil || !pred.predicate.Asserts {
		return ""
	}
	guard := c.detectTypeGuard(call)
	if guard == nil || strings.Contains(guard.VariableName, ".") {
		return ""
	}
	return guard.VariableName
}

package checker

import (
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/types"
)

// Function bodies are checked (Pass 3) before top-level initializers
// (Pass 5), so an unannotated top-level binding still has its Pass 2
// placeholder type, any, when a body reads it. A body read of such a binding
// therefore infers it on demand: the initializer is checked speculatively in
// the top-level context and its widened type replaces the placeholder. Pass 5
// still checks the initializer for real and reports its diagnostics.

// pendingInit is an unannotated top-level declarator whose initializer Pass 5
// hasn't checked yet.
type pendingInit struct {
	declarator *parser.VarDeclarator
	env        *Environment // the top-level scope it is declared in
	inProgress bool
	inferred   bool // the placeholder was replaced on demand
	blocked    bool // declared more than once (var); left alone
}

// registerPendingInit records a Pass 2 declarator for on-demand inference.
func (c *Checker) registerPendingInit(d *parser.VarDeclarator, env *Environment) {
	if d == nil || d.Name == nil || d.TypeAnnotation != nil || d.Value == nil {
		return
	}
	if _, isFn := d.Value.(*parser.FunctionLiteral); isFn {
		return
	}
	if c.pendingInits == nil {
		c.pendingInits = map[string]*pendingInit{}
	}
	if prev, dup := c.pendingInits[d.Name.Value]; dup {
		prev.blocked = true
		return
	}
	c.pendingInits[d.Name.Value] = &pendingInit{declarator: d, env: env}
}

// settlePendingInit drops a declarator once Pass 5 reaches it, restoring
// the placeholder so Pass 5 checks the initializer exactly as it would have.
func (c *Checker) settlePendingInit(name string) {
	if p := c.pendingInits[name]; p != nil && p.inferred {
		p.env.Update(name, types.Any)
	}
	delete(c.pendingInits, name)
}

// inferPendingInit gives a top-level binding read from inside a function its
// initializer's type, if it is still pending and the read isn't shadowed.
func (c *Checker) inferPendingInit(name string) {
	p := c.pendingInits[name]
	if p == nil || p.blocked || p.inProgress || c.functionNestingDepth == 0 {
		return
	}
	if c.definingEnv(name) != p.env {
		return // shadowed by a local
	}
	if t, _, ok := p.env.Resolve(name); !ok || t != types.Any {
		return // already refined
	}

	p.inProgress = true
	saved := c.saveFunctionContext()
	c.env = p.env
	initializer := p.declarator.Value
	var inferred types.Type
	c.speculate(func() {
		c.visit(initializer)
		if computed := initializer.GetComputedType(); computed != nil {
			inferred = c.widenedDeclarationType(initializer, computed, p.declarator.Name, p.env)
		}
	})
	c.restoreFunctionContext(saved)
	p.inProgress = false

	if inferred != nil && inferred != types.Any {
		p.env.Update(name, inferred)
		p.inferred = true
	}
}

// definingEnv is the nearest scope that declares name.
func (c *Checker) definingEnv(name string) *Environment {
	for e := c.env; e != nil; e = e.outer {
		if _, ok := e.symbols[name]; ok {
			return e
		}
	}
	return nil
}

// widenedDeclarationType is the type an unannotated declaration infers from
// its initializer (Pass 5's widening rules).
func (c *Checker) widenedDeclarationType(initializer parser.Expression, computed types.Type, name *parser.Identifier, env *Environment) types.Type {
	if arr, ok := computed.(*types.ArrayType); ok && arr.ElementType == types.Unknown {
		return computed
	}
	if isConstAssertion(initializer) {
		return computed
	}
	if _, isBareLiteral := computed.(*types.LiteralType); isBareLiteral && !isFreshLiteralExpression(initializer) {
		return computed
	}
	widened := types.DeeplyWidenType(computed)
	if !isConstVarLikeName(name, env) {
		widened = types.WidenEnumMember(widened)
	}
	return widened
}

// functionContext is the per-function checking state an on-demand
// top-level check must not inherit from the body it interrupts.
type functionContext struct {
	env                   *Environment
	expectedReturnType    types.Type
	inferredReturnTypes   []types.Type
	inferredYieldTypes    []types.Type
	thisType              types.Type
	inAsync, inGenerator  bool
	loopDepth, switchDep  int
	activeLabels          map[string]bool
	nestingDepth          int
	nonArrowFunctionDepth int
	crossFunctionTargets  bool
	flowNarrowOverlay     map[string]types.Type
}

func (c *Checker) saveFunctionContext() functionContext {
	s := functionContext{
		env:                   c.env,
		expectedReturnType:    c.currentExpectedReturnType,
		inferredReturnTypes:   c.currentInferredReturnTypes,
		inferredYieldTypes:    c.currentInferredYieldTypes,
		thisType:              c.currentThisType,
		inAsync:               c.inAsyncFunction,
		inGenerator:           c.inGeneratorFunction,
		loopDepth:             c.loopDepth,
		switchDep:             c.switchDepth,
		activeLabels:          c.activeLabels,
		nestingDepth:          c.functionNestingDepth,
		nonArrowFunctionDepth: c.nonArrowFunctionDepth,
		crossFunctionTargets:  c.crossFunctionTargets,
		flowNarrowOverlay:     c.flowNarrowOverlay,
	}
	c.currentExpectedReturnType = nil
	c.currentInferredReturnTypes = nil
	c.currentInferredYieldTypes = nil
	c.currentThisType = nil
	c.inAsyncFunction = false
	c.inGeneratorFunction = false
	c.loopDepth = 0
	c.switchDepth = 0
	c.activeLabels = map[string]bool{}
	c.functionNestingDepth = 0
	c.nonArrowFunctionDepth = 0
	c.crossFunctionTargets = false
	c.flowNarrowOverlay = nil
	return s
}

func (c *Checker) restoreFunctionContext(s functionContext) {
	c.env = s.env
	c.currentExpectedReturnType = s.expectedReturnType
	c.currentInferredReturnTypes = s.inferredReturnTypes
	c.currentInferredYieldTypes = s.inferredYieldTypes
	c.currentThisType = s.thisType
	c.inAsyncFunction = s.inAsync
	c.inGeneratorFunction = s.inGenerator
	c.loopDepth = s.loopDepth
	c.switchDepth = s.switchDep
	c.activeLabels = s.activeLabels
	c.functionNestingDepth = s.nestingDepth
	c.nonArrowFunctionDepth = s.nonArrowFunctionDepth
	c.crossFunctionTargets = s.crossFunctionTargets
	c.flowNarrowOverlay = s.flowNarrowOverlay
}

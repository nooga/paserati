package compiler

import (
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/nooga/paserati/pkg/errors"
	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/vm"
)

// A sloppy direct eval's var and function declarations land in the calling
// function's variable environment, creating bindings that code compiled
// before the eval ran knows nothing about (paserati#72):
//
//	function f() { eval("var x = 1"); return x }   // 1, even if x is global
//
// A sloppy function whose own body contains a direct eval therefore gets an
// "eval env": a null-prototype object, made fresh on every call and held in a
// hidden local, into which eval code puts the bindings it creates. Any
// reference compiled in (or inside) such a function whose name resolves
// outside it first looks for the name in the env objects in between,
// innermost first, and only then falls back to its static resolution. The
// env is an ordinary binding named evalEnvPrefix+N, so closures capture it
// and the scope descriptor hands it to eval code like any other local.

const evalEnvPrefix = "%evalenv"

var evalEnvCounter atomic.Int64

// containsOwnDirectEval reports whether a function body calls eval directly
// outside any nested function or class body (whose evals have their own
// variable environment).
func containsOwnDirectEval(body parser.Node) bool {
	found := false
	var walk func(n parser.Node)
	walk = func(n parser.Node) {
		if found || isNilASTNode(n) {
			return
		}
		switch x := n.(type) {
		case *parser.CallExpression:
			if x.IsDirectEval {
				found = true
				return
			}
		case *parser.FunctionLiteral, *parser.ArrowFunctionLiteral, *parser.ShorthandMethod,
			*parser.ClassBody:
			return
		}
		walkChildren(n, walk)
	}
	walk(body)
	return found
}

// setupEvalEnv gives a sloppy function containing direct eval its eval env.
// Called on the function's compiler before its body is compiled.
func (c *Compiler) setupEvalEnv(body parser.Node, line int) {
	if c.chunk.IsStrict || body == nil || !containsOwnDirectEval(body) {
		return
	}
	name := fmt.Sprintf("%s%d", evalEnvPrefix, evalEnvCounter.Add(1))
	reg := c.regAlloc.Alloc()
	c.regAlloc.Pin(reg)
	c.emitMakeEmptyObject(reg, line)
	nullReg := c.regAlloc.Alloc()
	c.emitLoadNull(nullReg, line)
	c.emitOpCode(vm.OpSetPrototype, line)
	c.emitByte(byte(reg))
	c.emitByte(byte(nullReg))
	c.regAlloc.Free(nullReg)
	c.currentSymbolTable.Define(name, reg)
	if c.allLocalNames == nil {
		c.allLocalNames = make(map[Register]string)
	}
	c.allLocalNames[reg] = name
	c.evalEnvSyms = []string{name}
}

// evalEnvNamesInScope lists the env bindings a direct eval here can see,
// innermost first: this compiler's own, then its enclosing compilers'.
func (c *Compiler) evalEnvNamesInScope() []string {
	var names []string
	for cc := c; cc != nil; cc = cc.enclosing {
		names = append(names, cc.evalEnvSyms...)
	}
	return names
}

// evalEnvsFor lists, innermost first, the env bindings a reference to name
// must consult before its static resolution: those of every function between
// here and the scope that statically declares name.
func (c *Compiler) evalEnvsFor(name string) []string {
	if strings.HasPrefix(name, "%") {
		return nil
	}
	sym, table, found := c.currentSymbolTable.Resolve(name)
	var envs []string
	for cc := c; cc != nil; cc = cc.enclosing {
		// Declared inside cc itself: eval cannot shadow it from cc's env.
		// (An eval compiler's caller upvalues are the caller's outer
		// bindings, which the caller's env can shadow.)
		if found && !sym.IsGlobal && !sym.IsCallerUpvalue && cc.isInCurrentScopeChain(table) {
			break
		}
		envs = append(envs, cc.evalEnvSyms...)
	}
	return envs
}

// emitLoadEvalEnv loads the env binding called name into dest.
func (c *Compiler) emitLoadEvalEnv(name string, dest Register, line int) {
	sym, table, found := c.currentSymbolTable.Resolve(name)
	if !found {
		c.emitLoadUndefined(dest, line)
		return
	}
	if c.isInCurrentScopeChain(table) || c.enclosing == nil {
		c.emitMove(dest, sym.Register, line)
		return
	}
	c.emitLoadFree(dest, c.addFreeSymbol(nil, &sym), line)
}

// emitFindEvalEnv sets dest to the innermost env object in envs that has an
// own property name, or to undefined.
func (c *Compiler) emitFindEvalEnv(name string, envs []string, dest Register, line int) {
	keyReg := c.regAlloc.Alloc()
	testReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(keyReg)
	defer c.regAlloc.Free(testReg)
	c.emitLoadNewConstant(keyReg, vm.String(name), line)
	var found []int
	for _, env := range envs {
		c.emitLoadEvalEnv(env, dest, line)
		c.emitIn(testReg, keyReg, dest, line)
		next := c.emitPlaceholderJump(vm.OpJumpIfFalse, testReg, line)
		found = append(found, c.emitPlaceholderJump(vm.OpJump, 0, line))
		c.patchJump(next)
	}
	c.emitLoadUndefined(dest, line)
	for _, j := range found {
		c.patchJump(j)
	}
}

// evalEnvTarget is the identifier a node reads or writes, if it is one an
// eval env may shadow.
func evalEnvTarget(node parser.Node) *parser.Identifier {
	switch n := node.(type) {
	case *parser.Identifier:
		return n
	case *parser.TypeofExpression:
		id, _ := n.Operand.(*parser.Identifier)
		return id
	case *parser.PrefixExpression:
		if n.Operator == "delete" {
			id, _ := n.Right.(*parser.Identifier)
			return id
		}
	case *parser.AssignmentExpression:
		id, _ := n.Left.(*parser.Identifier)
		return id
	case *parser.UpdateExpression:
		id, _ := n.Argument.(*parser.Identifier)
		return id
	}
	return nil
}

// withEnvMember returns node with its identifier replaced by envName.prop.
func withEnvMember(node parser.Node, id *parser.Identifier, envName string) parser.Node {
	member := &parser.MemberExpression{
		Token:    id.Token,
		Object:   &parser.Identifier{Token: id.Token, Value: envName},
		Property: &parser.Identifier{Token: id.Token, Value: id.Value},
	}
	switch n := node.(type) {
	case *parser.Identifier:
		return member
	case *parser.TypeofExpression:
		c := *n
		c.Operand = member
		return &c
	case *parser.PrefixExpression:
		c := *n
		c.Right = member
		return &c
	case *parser.AssignmentExpression:
		c := *n
		c.Left = member
		return &c
	case *parser.UpdateExpression:
		c := *n
		c.Argument = member
		return &c
	}
	return node
}

// compileViaEvalEnv compiles a reference an eval env may shadow as
//
//	env = <innermost env holding the name>
//	env !== undefined ? <node on env.name> : <node as usual>
//
// resolving the reference once, before any right-hand side runs. It reports
// false when node needs no such treatment.
func (c *Compiler) compileViaEvalEnv(node parser.Node, hint Register) (Register, bool, errors.PaseratiError) {
	id := evalEnvTarget(node)
	if id == nil {
		return BadRegister, false, nil
	}
	if c.evalEnvBypass == node {
		c.evalEnvBypass = nil
		return BadRegister, false, nil
	}
	envs := c.evalEnvsFor(id.Value)
	if len(envs) == 0 {
		return BadRegister, false, nil
	}
	if hint == BadRegister {
		hint = c.regAlloc.Alloc()
	}
	line := id.Token.Line
	envReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(envReg)
	c.emitFindEvalEnv(id.Value, envs, envReg, line)
	toStatic := c.emitPlaceholderJump(vm.OpJumpIfUndefined, envReg, line)

	envName := fmt.Sprintf("%%evalenvtarget%d", envReg)
	c.currentSymbolTable.Define(envName, envReg)
	res, err := c.compileNode(withEnvMember(node, id, envName), hint)
	delete(c.currentSymbolTable.store, envName)
	if err != nil {
		return BadRegister, true, err
	}
	if res != hint {
		c.emitMove(hint, res, line)
	}
	toEnd := c.emitPlaceholderJump(vm.OpJump, 0, line)

	c.patchJump(toStatic)
	c.evalEnvBypass = node
	res, err = c.compileNode(node, hint)
	if err != nil {
		return BadRegister, true, err
	}
	if res != hint {
		c.emitMove(hint, res, line)
	}
	c.patchJump(toEnd)
	return hint, true, nil
}

// storeViaEvalEnv emits the env half of storing valueReg into name: when an
// env holds name, store there and skip the static store that the caller
// emits next. It returns the jump to patch once the static store is emitted
// (or -1 when no env applies).
func (c *Compiler) storeViaEvalEnv(name string, valueReg Register, line int) int {
	envs := c.evalEnvsFor(name)
	if len(envs) == 0 {
		return -1
	}
	envReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(envReg)
	c.emitFindEvalEnv(name, envs, envReg, line)
	toStatic := c.emitPlaceholderJump(vm.OpJumpIfUndefined, envReg, line)
	c.emitSetProp(envReg, valueReg, c.chunk.AddConstant(vm.String(name)), line)
	toEnd := c.emitPlaceholderJump(vm.OpJump, 0, line)
	c.patchJump(toStatic)
	return toEnd
}

// hasEvalEnvInScope reports whether any function this code is nested in has
// an eval env, the only case in which references need the env check.
func (c *Compiler) hasEvalEnvInScope() bool {
	for cc := c; cc != nil; cc = cc.enclosing {
		if len(cc.evalEnvSyms) > 0 {
			return true
		}
	}
	return false
}

// evalHoistsToEnv reports whether this is sloppy direct eval code whose
// var and function declarations go into its caller's eval env.
func (c *Compiler) evalHoistsToEnv() bool {
	return c.enclosing == nil && c.callerScopeDesc != nil && !c.chunk.IsStrict && len(c.evalEnvSyms) > 0
}

// dropCallerUpvalueSymbol removes the pre-loaded symbol for a caller upvalue
// the eval code redeclares: its var shadows the outer binding, so references
// must find it in the env instead.
func (c *Compiler) dropCallerUpvalueSymbol(name string) {
	if sym, ok := c.currentSymbolTable.store[name]; ok && sym.IsCallerUpvalue {
		delete(c.currentSymbolTable.store, name)
	}
}

// callerOwnLocal returns the pre-loaded symbol for name when it is one of the
// caller function's own locals, which eval declarations reuse.
func (c *Compiler) callerOwnLocal(name string) (Symbol, bool) {
	sym, ok := c.currentSymbolTable.store[name]
	return sym, ok && sym.IsCallerLocal && !sym.IsCallerUpvalue
}

// hoistEvalVarToEnv instantiates a var of eval code in its caller's env:
// unless the caller already has the binding, create it as undefined.
func (c *Compiler) hoistEvalVarToEnv(name string) {
	if _, ok := c.callerOwnLocal(name); ok {
		return
	}
	c.dropCallerUpvalueSymbol(name)
	envReg := c.regAlloc.Alloc()
	keyReg := c.regAlloc.Alloc()
	testReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(envReg)
	defer c.regAlloc.Free(keyReg)
	defer c.regAlloc.Free(testReg)
	c.emitLoadEvalEnv(c.evalEnvSyms[0], envReg, 0)
	c.emitLoadNewConstant(keyReg, vm.String(name), 0)
	c.emitIn(testReg, keyReg, envReg, 0)
	missing := c.emitPlaceholderJump(vm.OpJumpIfFalse, testReg, 0)
	exists := c.emitPlaceholderJump(vm.OpJump, 0, 0)
	c.patchJump(missing)
	c.emitLoadUndefined(testReg, 0)
	c.emitSetProp(envReg, testReg, c.chunk.AddConstant(vm.String(name)), 0)
	c.patchJump(exists)
}

// hoistEvalFunctionToEnv instantiates a function declaration of eval code:
// into the caller's own local of that name, or else its env.
func (c *Compiler) hoistEvalFunctionToEnv(name string, funcLit *parser.FunctionLiteral) {
	funcConstIndex, freeSymbols, err := c.compileFunctionLiteral(funcLit, name)
	if err != nil {
		return
	}
	line := funcLit.Token.Line
	closureReg := c.emitClosure(c.regAlloc.Alloc(), funcConstIndex, funcLit, freeSymbols)
	defer c.regAlloc.Free(closureReg)
	if sym, ok := c.callerOwnLocal(name); ok {
		c.emitMove(sym.Register, closureReg, line)
		c.emitSetCallerBinding(sym.CallerLocalIndex, false, closureReg, line)
		return
	}
	envReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(envReg)
	c.emitLoadEvalEnv(c.evalEnvSyms[0], envReg, line)
	c.emitSetProp(envReg, closureReg, c.chunk.AddConstant(vm.String(name)), line)
}

// isSloppyDirectEvalTopLevel reports whether this compiles the top level of
// sloppy direct eval code, whose vars belong to the caller's variable
// environment rather than to the eval.
func (c *Compiler) isSloppyDirectEvalTopLevel() bool {
	return c.enclosing == nil && c.callerScopeDesc != nil && !c.chunk.IsStrict
}

// compileEvalCalleeFromEnv compiles eval(...) as
//
//	env = <innermost env holding "eval">
//	env !== undefined ? (0, env.eval)(...) : <direct eval>
//
// when an eval env may hold a binding named eval. It reports false otherwise.
func (c *Compiler) compileEvalCalleeFromEnv(node *parser.CallExpression, hint Register) (Register, bool, errors.PaseratiError) {
	if c.evalEnvBypass == node {
		c.evalEnvBypass = nil
		return BadRegister, false, nil
	}
	ident, ok := node.Function.(*parser.Identifier)
	if !ok {
		return BadRegister, false, nil
	}
	envs := c.evalEnvsFor(ident.Value)
	if len(envs) == 0 {
		return BadRegister, false, nil
	}
	line := node.Token.Line
	envReg := c.regAlloc.Alloc()
	calleeReg := c.regAlloc.Alloc()
	defer c.regAlloc.Free(envReg)
	defer c.regAlloc.Free(calleeReg)
	c.emitFindEvalEnv(ident.Value, envs, envReg, line)
	toDirect := c.emitPlaceholderJump(vm.OpJumpIfUndefined, envReg, line)

	c.emitGetProp(calleeReg, envReg, c.chunk.AddConstant(vm.String(ident.Value)), line)
	calleeName := fmt.Sprintf("%%evalcallee%d", calleeReg)
	c.currentSymbolTable.Define(calleeName, calleeReg)
	call := *node
	call.IsDirectEval = false
	call.Function = &parser.Identifier{Token: ident.Token, Value: calleeName}
	res, err := c.compileNode(&call, hint)
	delete(c.currentSymbolTable.store, calleeName)
	if err != nil {
		return BadRegister, true, err
	}
	if res != hint {
		c.emitMove(hint, res, line)
	}
	toEnd := c.emitPlaceholderJump(vm.OpJump, 0, line)

	c.patchJump(toDirect)
	c.evalEnvBypass = node
	res, err = c.compileNode(node, hint)
	if err != nil {
		return BadRegister, true, err
	}
	if res != hint {
		c.emitMove(hint, res, line)
	}
	c.patchJump(toEnd)
	return hint, true, nil
}

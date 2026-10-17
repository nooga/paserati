package compiler

import (
	"strings"

	"github.com/nooga/paserati/pkg/vm"
)

// Direct eval code resolves names in the calling function's scope chain, but
// closures here only capture the outer bindings their own source mentions. A
// function containing direct eval therefore captures every binding visible to
// it from enclosing functions, and its ScopeDescriptor names them by upvalue
// index so the eval compiler can reach them with OpGetCallerUpvalue and
// OpSetCallerUpvalue.

// captureVisibleForEval adds every non-global binding visible in the enclosing
// compiler c (at the point fc's function is defined) to fc's free symbols.
func (c *Compiler) captureVisibleForEval(fc *Compiler) {
	seen := make(map[string]bool)
	for table := c.currentSymbolTable; table != nil; table = table.Outer {
		for name := range table.store {
			if seen[name] {
				continue
			}
			seen[name] = true
			sym, _, found := c.currentSymbolTable.Resolve(name)
			if !found || !capturableForEval(name, sym) {
				continue
			}
			fc.addFreeSymbol(nil, &sym)
		}
	}
}

// capturableForEval reports whether sym is a real binding a closure can
// capture: not a global (eval reaches those itself), not a namespace member,
// and not a placeholder that has no storage yet.
func capturableForEval(name string, sym Symbol) bool {
	if name == "" || sym.IsGlobal || sym.IsNamespaceProperty {
		return false
	}
	if sym.Register == nilRegister && !sym.IsSpilled {
		return false
	}
	if strings.HasPrefix(name, evalEnvPrefix) {
		return true // outer functions' eval envs (compile_eval_env.go)
	}
	for _, r := range name {
		if !(r == '_' || r == '$' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r > 127) {
			return false // compiler-internal names
		}
	}
	return true
}

// upvalueNames lists the names of this function's free symbols by upvalue index.
func (c *Compiler) upvalueNames() []string {
	names := make([]string, len(c.freeSymbols))
	for i, sym := range c.freeSymbols {
		names[i] = sym.Name
	}
	return names
}

// resolveCallerUpvalue returns the caller upvalue index eval code should use
// for name, or -1. The caller's own locals shadow its upvalues.
func (c *Compiler) resolveCallerUpvalue(name string) int {
	if c.callerScopeDesc == nil || c.resolveCallerLocal(name) >= 0 {
		return -1
	}
	for i, n := range c.callerScopeDesc.UpvalueNames {
		if n == name {
			return i
		}
	}
	return -1
}

// emitGetCallerBinding loads a caller local (register index) or caller
// upvalue into dest.
func (c *Compiler) emitGetCallerBinding(dest Register, index int, isUpvalue bool, line int) {
	if isUpvalue {
		c.emitOpCode(vm.OpGetCallerUpvalue, line)
		c.emitByte(byte(dest))
		c.emitUint16(uint16(index))
		return
	}
	c.emitOpCode(vm.OpGetCallerLocal, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(index))
}

// emitSetCallerBinding stores src into a caller local or caller upvalue.
func (c *Compiler) emitSetCallerBinding(index int, isUpvalue bool, src Register, line int) {
	if isUpvalue {
		c.emitOpCode(vm.OpSetCallerUpvalue, line)
		c.emitUint16(uint16(index))
		c.emitByte(byte(src))
		return
	}
	c.emitOpCode(vm.OpSetCallerLocal, line)
	c.emitByte(byte(index))
	c.emitByte(byte(src))
}

// unresolvedCallerBinding resolves a name the symbol table did not find
// against the eval caller's locals, then its upvalues.
func (c *Compiler) unresolvedCallerBinding(name string, found bool) (index int, isUpvalue bool, ok bool) {
	if found || c.callerScopeDesc == nil {
		return 0, false, false
	}
	if idx := c.resolveCallerLocal(name); idx >= 0 {
		return idx, false, true
	}
	if idx := c.resolveCallerUpvalue(name); idx >= 0 {
		return idx, true, true
	}
	return 0, false, false
}

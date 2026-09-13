package compiler

import (
	"fmt"

	"github.com/nooga/paserati/pkg/parser"
	"github.com/nooga/paserati/pkg/vm"
)

// This file holds the shared machinery for binding a function's parameters
// (including its rest parameter) to either a register or a spill slot -
// paserati#467, the callee-side counterpart to paserati#461's caller-side
// fix. #461 removed the register ceiling on how many arguments a call site
// can pass; this removes the matching ceiling on how many named parameters
// a function declaration can bind, by reusing the spill-slot mechanism
// already built for local variables (compiler.go's AllocSpillSlot,
// SymbolTable.DefineSpilled, emitLoadSpill/emitStoreSpill) instead of
// inventing a second, array-based calling convention - see the paserati#467
// design discussion for why.
//
// Every function-literal compiler (arrow, plain function, method, shorthand
// method - four near-identical parameter-binding loops, one per file/
// function) calls into defineParamOrSpill/emitParamDefault below instead of
// repeating this branch inline.

// defineParamOrSpill binds one parameter name to a register, or - once the
// register budget reserved for parameters (regalloc.go's
// ParamRegisterThreshold) is exhausted - to a spill slot. Returns the symbol
// that was defined and whether a register was used.
//
// A register-bound parameter is pinned (parameters can be captured by inner
// closures via CaptureFromRegister); a spilled one needs no pin, since spill
// slots aren't part of the register free list and can't be reclaimed out
// from under it the way a register could.
//
// Below ParamRegisterThreshold (the overwhelming majority of real
// functions), this is exactly the old `regAlloc.Alloc()` +
// `currentSymbolTable.Define()` + `regAlloc.Pin()` sequence - same
// registers, same order, zero behavior change. Only once a function
// declares more parameters than fit does the spill branch ever run.
func (c *Compiler) defineParamOrSpill(name string) (sym Symbol, usedRegister bool) {
	if reg, ok := c.regAlloc.TryAllocForParam(); ok {
		sym = c.currentSymbolTable.Define(name, reg)
		c.regAlloc.Pin(reg)
		return sym, true
	}
	spillIdx := c.AllocSpillSlot()
	sym = c.currentSymbolTable.DefineSpilled(name, spillIdx)
	return sym, false
}

// reserveRestParamSlot mirrors defineParamOrSpill for the rest parameter's
// storage, without defining its name yet - callers reserve this slot
// immediately after named parameters (before any default-value/
// destructuring temp register can be allocated and freed in between; see
// the #443 investigation referenced throughout the four call sites) and
// only bind the name once the rest parameter's own pattern is known.
func (c *Compiler) reserveRestParamSlot() (reg Register, spillIdx uint16, isSpilled bool) {
	if r, ok := c.regAlloc.TryAllocForParam(); ok {
		c.regAlloc.Pin(r)
		return r, 0, false
	}
	spillIdx = c.AllocSpillSlot()
	return 0, spillIdx, true
}

// emitParamDefault compiles `if (param === undefined) param = defaultExpr`
// for one already-bound parameter symbol, whether it lives in a register or
// a spill slot. tdzIndex is forwarded to currentDefaultParamIndex exactly as
// the original inline per-site code did, for TDZ tracking against
// parameters later in the list.
func (c *Compiler) emitParamDefault(sym Symbol, defaultExpr parser.Expression, tdzIndex int, line int, errNode parser.Node, paramName string) {
	var curReg Register
	if sym.IsSpilled {
		curReg = c.regAlloc.Alloc()
		c.emitLoadSpill(curReg, sym.SpillIndex, line)
	} else {
		curReg = sym.Register
	}

	undefinedReg := c.regAlloc.Alloc()
	c.emitLoadUndefined(undefinedReg, line)
	compareReg := c.regAlloc.Alloc()
	c.emitStrictEqual(compareReg, curReg, undefinedReg, line)
	jumpIfDefinedPos := c.emitPlaceholderJump(vm.OpJumpIfFalse, compareReg, line)
	c.regAlloc.Free(undefinedReg)
	c.regAlloc.Free(compareReg)

	defaultValueReg := c.regAlloc.Alloc()
	c.currentDefaultParamIndex = tdzIndex
	c.inDefaultParamScope = true
	_, err := c.compileNode(defaultExpr, defaultValueReg)
	c.inDefaultParamScope = false
	c.currentDefaultParamIndex = -1
	if err != nil {
		c.addError(errNode, fmt.Sprintf("error compiling default value for parameter %s", paramName))
	} else if sym.IsSpilled {
		c.emitStoreSpill(sym.SpillIndex, defaultValueReg, line)
	} else if defaultValueReg != curReg {
		c.emitMove(curReg, defaultValueReg, line)
	}
	c.regAlloc.Free(defaultValueReg)
	if sym.IsSpilled {
		c.regAlloc.Free(curReg)
	}

	c.patchJump(jumpIfDefinedPos)
}

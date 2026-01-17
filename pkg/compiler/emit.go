package compiler

import (
	"fmt"

	"github.com/nooga/paserati/pkg/vm"
)

// --- Bytecode Emission Helpers ---

func (c *Compiler) emitOpCode(op vm.OpCode, line int) {
	c.chunk.WriteOpCode(op, line)
}

func (c *Compiler) emitByte(b byte) {
	c.chunk.EmitByte(b)
}

func (c *Compiler) emitUint16(val uint16) {
	c.chunk.WriteUint16(val)
}

func (c *Compiler) emitLoadConstant(dest Register, constIdx uint16, line int) {
	c.emitOpCode(vm.OpLoadConst, line)
	c.emitByte(byte(dest))
	c.emitUint16(constIdx)
}

func (c *Compiler) emitLoadNull(dest Register, line int) {
	// fmt.Printf("[EMIT DEBUG] emitLoadNull called with dest=R%d, line=%d\n", dest, line)
	c.emitOpCode(vm.OpLoadNull, line)
	c.emitByte(byte(dest))
}

func (c *Compiler) emitLoadUndefined(dest Register, line int) {
	c.emitOpCode(vm.OpLoadUndefined, line)
	c.emitByte(byte(dest))
}

func (c *Compiler) emitLoadTrue(dest Register, line int) {
	c.emitOpCode(vm.OpLoadTrue, line)
	c.emitByte(byte(dest))
}

func (c *Compiler) emitLoadFalse(dest Register, line int) {
	c.emitOpCode(vm.OpLoadFalse, line)
	c.emitByte(byte(dest))
}

func (c *Compiler) emitMove(dest, src Register, line int) {
	if dest == src {
		return // Skip redundant move
	}
	c.emitOpCode(vm.OpMove, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(src))
}

// emitLoadSpill loads a value from a spill slot into a register.
// Used when accessing a spilled variable.
// Uses 8-bit opcode for indices 0-255, 16-bit opcode for larger indices.
func (c *Compiler) emitLoadSpill(dest Register, spillIdx uint16, line int) {
	if spillIdx <= 255 {
		c.emitOpCode(vm.OpLoadSpill, line)
		c.emitByte(byte(dest))
		c.emitByte(byte(spillIdx))
	} else {
		c.emitOpCode(vm.OpLoadSpill16, line)
		c.emitByte(byte(dest))
		c.emitByte(byte(spillIdx >> 8))   // High byte
		c.emitByte(byte(spillIdx & 0xFF)) // Low byte
	}
}

// emitStoreSpill stores a register value into a spill slot.
// Used when initializing or assigning to a spilled variable.
// Uses 8-bit opcode for indices 0-255, 16-bit opcode for larger indices.
func (c *Compiler) emitStoreSpill(spillIdx uint16, src Register, line int) {
	if spillIdx <= 255 {
		c.emitOpCode(vm.OpStoreSpill, line)
		c.emitByte(byte(spillIdx))
		c.emitByte(byte(src))
	} else {
		c.emitOpCode(vm.OpStoreSpill16, line)
		c.emitByte(byte(spillIdx >> 8))   // High byte
		c.emitByte(byte(spillIdx & 0xFF)) // Low byte
		c.emitByte(byte(src))
	}
}

func (c *Compiler) emitReturn(src Register, line int) {
	c.emitOpCode(vm.OpReturn, line)
	c.emitByte(byte(src))
}

// emitReturnFinally emits OpReturnFinally for returns in finally blocks
func (c *Compiler) emitReturnFinally(src Register, line int) {
	c.emitOpCode(vm.OpReturnFinally, line)
	c.emitByte(byte(src))
}

// emitHandlePendingAction emits OpHandlePending to handle pending actions after finally
func (c *Compiler) emitHandlePendingAction(line int) {
	c.emitOpCode(vm.OpHandlePending, line)
}

func (c *Compiler) emitNegate(dest, src Register, line int) {
	c.emitOpCode(vm.OpNegate, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(src))
}

func (c *Compiler) emitNot(dest, src Register, line int) {
	c.emitOpCode(vm.OpNot, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(src))
}

func (c *Compiler) emitTypeof(dest, src Register, line int) {
	c.emitOpCode(vm.OpTypeof, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(src))
}

func (c *Compiler) emitTypeofIdentifier(dest Register, identifierName string, line int) {
	// Add identifier name as a constant and emit OpTypeofIdentifier
	nameIdx := c.chunk.AddConstant(vm.String(identifierName))
	c.emitOpCode(vm.OpTypeofIdentifier, line)
	c.emitByte(byte(dest))
	c.emitUint16(nameIdx)
}

func (c *Compiler) emitAdd(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpAdd, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitSubtract(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpSubtract, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitMultiply(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpMultiply, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitDivide(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpDivide, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitStringConcat(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpStringConcat, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitEqual(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpEqual, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitNotEqual(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpNotEqual, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitGreater(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpGreater, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitLess(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpLess, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitLessEqual(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpLessEqual, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitGreaterEqual(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpGreaterEqual, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitIn(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpIn, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitInstanceof(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpInstanceof, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitCall(dest, funcReg Register, argCount byte, line int) {
	// Validate that dest register is within allocated range
	maxReg := c.regAlloc.MaxRegs()

	// DEBUG: Print all OpCall emissions, especially those with small maxReg
	if debugRegAlloc && maxReg <= 10 {
		fmt.Printf("[EMIT_CALL] dest=R%d funcReg=R%d argCount=%d maxReg=%d line=%d func=%s\n",
			dest, funcReg, argCount, maxReg, line, c.compilingFuncName)
	}

	if dest >= maxReg {
		fmt.Printf("[EMIT CALL BUG] Emitting OpCall with dest=R%d but function only has %d registers! funcReg=R%d argCount=%d line=%d func=%s\n",
			dest, maxReg, funcReg, argCount, line, c.compilingFuncName)
	}
	c.emitOpCode(vm.OpCall, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(funcReg))
	c.emitByte(argCount)
}

func (c *Compiler) emitTailCall(dest, funcReg Register, argCount byte, line int) {
	c.emitOpCode(vm.OpTailCall, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(funcReg))
	c.emitByte(argCount)
}

func (c *Compiler) emitTailCallMethod(dest, funcReg, thisReg Register, argCount byte, line int) {
	c.emitOpCode(vm.OpTailCallMethod, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(funcReg))
	c.emitByte(byte(thisReg))
	c.emitByte(argCount)
}

// emitCallMethod emits OpCallMethod with method call convention (this as implicit first parameter)
func (c *Compiler) emitCallMethod(dest, funcReg, thisReg Register, argCount byte, line int) {
	// Validate that dest register is within allocated range
	maxReg := c.regAlloc.MaxRegs()

	// DEBUG: Print all OpCallMethod emissions, especially those with small maxReg
	if debugRegAlloc && maxReg <= 10 {
		fmt.Printf("[EMIT_CALLMETHOD] dest=R%d funcReg=R%d thisReg=R%d argCount=%d maxReg=%d line=%d func=%s\n",
			dest, funcReg, thisReg, argCount, maxReg, line, c.compilingFuncName)
	}

	if dest >= maxReg {
		fmt.Printf("[EMIT CALLMETHOD BUG] Emitting OpCallMethod with dest=R%d but function only has %d registers! funcReg=R%d thisReg=R%d argCount=%d line=%d func=%s\n",
			dest, maxReg, funcReg, thisReg, argCount, line, c.compilingFuncName)
	}
	c.emitOpCode(vm.OpCallMethod, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(funcReg))
	c.emitByte(byte(thisReg))
	c.emitByte(argCount)
}

// emitSpreadCall emits OpSpreadCall for function calls with spread arguments
func (c *Compiler) emitSpreadCall(dest, funcReg, spreadArgReg Register, line int) {
	c.emitOpCode(vm.OpSpreadCall, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(funcReg))
	c.emitByte(byte(spreadArgReg))
}

// emitSpreadCallMethod emits OpSpreadCallMethod for method calls with spread arguments
func (c *Compiler) emitSpreadCallMethod(dest, funcReg, thisReg, spreadArgReg Register, line int) {
	c.emitOpCode(vm.OpSpreadCallMethod, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(funcReg))
	c.emitByte(byte(thisReg))
	c.emitByte(byte(spreadArgReg))
}

// emitNew emits OpNew with constructor register, argument count, and flags
// If inheritNewTarget is true, inherits new.target from the caller (for super() calls)
func (c *Compiler) emitNew(dest, constructorReg Register, argCount byte, inheritNewTarget bool, line int) {
	c.emitOpCode(vm.OpNew, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(constructorReg))
	c.emitByte(argCount)
	var flags byte
	if inheritNewTarget {
		flags |= 0x01 // Bit 0: inherit new.target from caller
	}
	c.emitByte(flags)
}

// emitSpreadNew emits OpSpreadNew for constructor calls with spread arguments
// If inheritNewTarget is true, inherits new.target from the caller (for super() calls)
func (c *Compiler) emitSpreadNew(dest, constructorReg, spreadArgReg Register, inheritNewTarget bool, line int) {
	c.emitOpCode(vm.OpSpreadNew, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(constructorReg))
	c.emitByte(byte(spreadArgReg))
	var flags byte
	if inheritNewTarget {
		flags |= 0x01 // Bit 0: inherit new.target from caller
	}
	c.emitByte(flags)
}

// emitLoadThis emits OpLoadThis to load 'this' value from current call context
func (c *Compiler) emitLoadThis(dest Register, line int) {
	c.emitOpCode(vm.OpLoadThis, line)
	c.emitByte(byte(dest))
}

// emitSetThis emits OpSetThis to set the 'this' value in the current call frame context
func (c *Compiler) emitSetThis(src Register, line int) {
	c.emitOpCode(vm.OpSetThis, line)
	c.emitByte(byte(src))
}

// emitLoadNewTarget emits OpLoadNewTarget to load new.target value from current constructor call context
func (c *Compiler) emitLoadNewTarget(dest Register, line int) {
	c.emitOpCode(vm.OpLoadNewTarget, line)
	c.emitByte(byte(dest))
}

// emitGetSuperConstructor emits OpGetSuperConstructor to dynamically get the super constructor
// (the [[Prototype]] of the currently executing function) for super() calls
func (c *Compiler) emitGetSuperConstructor(dest Register, line int) {
	c.emitOpCode(vm.OpGetSuperConstructor, line)
	c.emitByte(byte(dest))
}

// emitLoadImportMeta emits OpLoadImportMeta to load import.meta object from current module context
func (c *Compiler) emitLoadImportMeta(dest Register, line int) {
	c.emitOpCode(vm.OpLoadImportMeta, line)
	c.emitByte(byte(dest))
}

// emitDynamicImport emits OpDynamicImport to dynamically import a module at runtime
// dest: register to store the imported module namespace
// specifierReg: register containing the module specifier string
func (c *Compiler) emitDynamicImport(dest Register, specifierReg Register, line int) {
	c.emitOpCode(vm.OpDynamicImport, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(specifierReg))
}

// emitGetArguments emits OpGetArguments to create arguments object from current function arguments
func (c *Compiler) emitGetArguments(dest Register, line int) {
	c.emitOpCode(vm.OpGetArguments, line)
	c.emitByte(byte(dest))
}

// emitFinalReturn adds the final OpReturnUndefined instruction.
func (c *Compiler) emitFinalReturn(line int) {
	// No need to load undefined first
	c.emitOpCode(vm.OpReturnUndefined, line)
}

// Overload or new function to handle adding constant and emitting load
func (c *Compiler) emitLoadNewConstant(dest Register, val vm.Value, line int) {
	constIdx := c.chunk.AddConstant(val)
	c.emitLoadConstant(dest, constIdx, line)
}

// Added Helper
func (c *Compiler) emitStrictEqual(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpStrictEqual, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

// Added Helper
func (c *Compiler) emitStrictNotEqual(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpStrictNotEqual, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

// Added helper for OpSetUpvalue (supports both 8-bit and 16-bit indices)
func (c *Compiler) emitSetUpvalue(upvalueIndex uint16, srcReg Register, line int) {
	if upvalueIndex <= 255 {
		c.emitOpCode(vm.OpSetUpvalue, line)
		c.emitByte(byte(upvalueIndex))
		c.emitByte(byte(srcReg))
	} else {
		c.emitOpCode(vm.OpSetUpvalue16, line)
		c.emitByte(byte(upvalueIndex >> 8))
		c.emitByte(byte(upvalueIndex & 0xFF))
		c.emitByte(byte(srcReg))
	}
}

// emitLoadFree loads a free variable (upvalue) into a register.
// Uses 8-bit opcode for indices 0-255, 16-bit opcode for larger indices.
func (c *Compiler) emitLoadFree(dest Register, upvalueIndex uint16, line int) {
	if upvalueIndex <= 255 {
		c.emitOpCode(vm.OpLoadFree, line)
		c.emitByte(byte(dest))
		c.emitByte(byte(upvalueIndex))
	} else {
		c.emitOpCode(vm.OpLoadFree16, line)
		c.emitByte(byte(dest))
		c.emitByte(byte(upvalueIndex >> 8))
		c.emitByte(byte(upvalueIndex & 0xFF))
	}
}

// --- NEW: emitGetLength ---
func (c *Compiler) emitGetLength(dest, src Register, line int) {
	c.emitOpCode(vm.OpGetLength, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(src))
}

// --- NEW: emitRemainder and emitExponent ---
func (c *Compiler) emitRemainder(dest, left, right Register, line int) {
	// REMOVED: c.stats.BytesGenerated += 4
	c.emitOpCode(vm.OpRemainder, line) // Fixed: Use emitOpCode
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitExponent(dest, left, right Register, line int) {
	// REMOVED: c.stats.BytesGenerated += 4
	c.emitOpCode(vm.OpExponent, line) // Fixed: Use emitOpCode
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

// --- END NEW ---

// --- NEW: Bitwise/Shift Emit Helpers ---

func (c *Compiler) emitBitwiseNot(dest, src Register, line int) {
	c.emitOpCode(vm.OpBitwiseNot, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(src))
}

func (c *Compiler) emitBitwiseAnd(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpBitwiseAnd, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitBitwiseOr(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpBitwiseOr, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitBitwiseXor(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpBitwiseXor, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitShiftLeft(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpShiftLeft, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitShiftRight(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpShiftRight, line) // Arithmetic shift
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

func (c *Compiler) emitUnsignedShiftRight(dest, left, right Register, line int) {
	c.emitOpCode(vm.OpUnsignedShiftRight, line) // Logical shift
	c.emitByte(byte(dest))
	c.emitByte(byte(left))
	c.emitByte(byte(right))
}

// emitMakeEmptyObject emits OpMakeEmptyObject DestReg
func (c *Compiler) emitMakeEmptyObject(dest Register, line int) {
	c.emitOpCode(vm.OpMakeEmptyObject, line) // Use the placeholder opcode
	c.emitByte(byte(dest))
}

// emitGetProp emits OpGetProp DestReg, ObjReg, NameConstIdx(Uint16)
func (c *Compiler) emitGetProp(dest, obj Register, nameConstIdx uint16, line int) {
	c.emitOpCode(vm.OpGetProp, line) // Use the placeholder opcode
	c.emitByte(byte(dest))
	c.emitByte(byte(obj))
	c.emitUint16(nameConstIdx)
}

// emitSetProp emits OpSetProp ObjReg, ValueReg, NameConstIdx(Uint16)
// Note: The order ObjReg, ValueReg, NameIdx seems reasonable for VM stack manipulation.
func (c *Compiler) emitSetProp(obj, val Register, nameConstIdx uint16, line int) {
	c.emitOpCode(vm.OpSetProp, line)
	c.emitByte(byte(obj))
	c.emitByte(byte(val))
	c.emitUint16(nameConstIdx)
}

// emitDefineDataProperty emits OpDefineDataProperty ObjReg, ValueReg, NameConstIdx(Uint16)
// This uses DefineOwnProperty semantics and can overwrite any existing property including accessors.
// Used for object literal data properties.
func (c *Compiler) emitDefineDataProperty(obj, val Register, nameConstIdx uint16, line int) {
	c.emitOpCode(vm.OpDefineDataProperty, line)
	c.emitByte(byte(obj))
	c.emitByte(byte(val))
	c.emitUint16(nameConstIdx)
}

// emitGetPrivateField emits OpGetPrivateField DestReg, ObjReg, NameConstIdx(Uint16)
// For ECMAScript private field access: obj.#field
func (c *Compiler) emitGetPrivateField(dest, obj Register, nameConstIdx uint16, line int) {
	c.emitOpCode(vm.OpGetPrivateField, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(obj))
	c.emitUint16(nameConstIdx)
}

// emitSetPrivateField emits OpSetPrivateField ObjReg, ValueReg, NameConstIdx(Uint16)
// For ECMAScript private field assignment: obj.#field = value
func (c *Compiler) emitSetPrivateField(obj, val Register, nameConstIdx uint16, line int) {
	c.emitOpCode(vm.OpSetPrivateField, line)
	c.emitByte(byte(obj))
	c.emitByte(byte(val))
	c.emitUint16(nameConstIdx)
}

// emitSetPrivateMethod emits OpSetPrivateMethod ObjReg, ValueReg, NameConstIdx(Uint16)
// For ECMAScript private method definition: obj.#method = func (not writable)
func (c *Compiler) emitSetPrivateMethod(obj, val Register, nameConstIdx uint16, line int) {
	c.emitOpCode(vm.OpSetPrivateMethod, line)
	c.emitByte(byte(obj))
	c.emitByte(byte(val))
	c.emitUint16(nameConstIdx)
}

// emitHasPrivateField emits OpHasPrivateField DestReg, ObjReg, NameConstIdx(Uint16)
// For ECMAScript private field presence check: #field in obj
func (c *Compiler) emitHasPrivateField(dest, obj Register, nameConstIdx uint16, line int) {
	c.emitOpCode(vm.OpHasPrivateField, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(obj))
	c.emitUint16(nameConstIdx)
}

// emitDefineMethod emits OpDefineMethod ObjReg, ValueReg, NameConstIdx(Uint16)
// Used for defining non-enumerable methods (e.g., class methods)
func (c *Compiler) emitDefineMethod(obj, val Register, nameConstIdx uint16, line int) {
	c.emitOpCode(vm.OpDefineMethod, line)
	c.emitByte(byte(obj))
	c.emitByte(byte(val))
	c.emitUint16(nameConstIdx)
}

// emitDefineMethodEnumerable emits OpDefineMethodEnumerable ObjReg, ValueReg, NameConstIdx(Uint16)
// Used for defining enumerable methods (e.g., object literal methods)
func (c *Compiler) emitDefineMethodEnumerable(obj, val Register, nameConstIdx uint16, line int) {
	c.emitOpCode(vm.OpDefineMethodEnumerable, line)
	c.emitByte(byte(obj))
	c.emitByte(byte(val))
	c.emitUint16(nameConstIdx)
}

// emitDefineMethodComputedEnumerable emits OpDefineMethodComputedEnumerable ObjReg, ValueReg, KeyReg
// Used for defining enumerable methods with computed keys (e.g., object literal computed methods)
func (c *Compiler) emitDefineMethodComputedEnumerable(obj, val, key Register, line int) {
	c.emitOpCode(vm.OpDefineMethodComputedEnumerable, line)
	c.emitByte(byte(obj))
	c.emitByte(byte(val))
	c.emitByte(byte(key))
}

// emitDefineAccessor emits OpDefineAccessor ObjReg, GetterReg, SetterReg, NameConstIdx(Uint16)
func (c *Compiler) emitDefineAccessor(obj, getter, setter Register, nameConstIdx uint16, line int) {
	c.emitOpCode(vm.OpDefineAccessor, line)
	c.emitByte(byte(obj))
	c.emitByte(byte(getter))
	c.emitByte(byte(setter))
	c.emitUint16(nameConstIdx)
}

// emitDefineAccessorDynamic emits OpDefineAccessorDynamic ObjReg, GetterReg, SetterReg, NameReg
func (c *Compiler) emitDefineAccessorDynamic(obj, getter, setter, nameReg Register, line int) {
	c.emitOpCode(vm.OpDefineAccessorDynamic, line)
	c.emitByte(byte(obj))
	c.emitByte(byte(getter))
	c.emitByte(byte(setter))
	c.emitByte(byte(nameReg))
}

// emitDeleteProp emits OpDeleteProp DestReg, ObjReg, NameConstIdx(Uint16)
func (c *Compiler) emitDeleteProp(dest, obj Register, nameConstIdx uint16, line int) {
	c.emitOpCode(vm.OpDeleteProp, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(obj))
	c.emitUint16(nameConstIdx)
}

// emitDeleteIndex emits OpDeleteIndex DestReg, ObjReg, KeyReg
func (c *Compiler) emitDeleteIndex(dest, obj, key Register, line int) {
	c.emitOpCode(vm.OpDeleteIndex, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(obj))
	c.emitByte(byte(key))
}

// --- END REVISED/NEW ---

// --- NEW: Global Variable Emit Functions ---

// emitGetGlobal emits OpGetGlobal instruction with direct global index
func (c *Compiler) emitGetGlobal(dest Register, globalIdx uint16, line int) {
	c.emitOpCode(vm.OpGetGlobal, line)
	c.emitByte(byte(dest))
	c.emitUint16(globalIdx)
}

// emitSetGlobal emits OpSetGlobal instruction with direct global index
func (c *Compiler) emitSetGlobal(globalIdx uint16, src Register, line int) {
	c.emitOpCode(vm.OpSetGlobal, line)
	c.emitUint16(globalIdx)
	c.emitByte(byte(src))
}

// emitToNumber implements unary plus by converting the operand to a number
func (c *Compiler) emitToNumber(dest, src Register, line int) {
	c.emitOpCode(vm.OpToNumber, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(src))
}

// emitToNumeric converts to Number but preserves BigInt (for ++/--)
func (c *Compiler) emitToNumeric(dest, src Register, line int) {
	c.emitOpCode(vm.OpToNumeric, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(src))
}

// emitLoadNumericOne loads 1 or 1n based on src type (for ++/--)
func (c *Compiler) emitLoadNumericOne(dest, src Register, line int) {
	c.emitOpCode(vm.OpLoadNumericOne, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(src))
}

// --- NEW: Efficient Nullish Check Emitters ---

// emitIsNull emits OpIsNull instruction: dest = (src === null)
func (c *Compiler) emitIsNull(dest, src Register, line int) {
	c.emitOpCode(vm.OpIsNull, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(src))
}

// emitIsUndefined emits OpIsUndefined instruction: dest = (src === undefined)
func (c *Compiler) emitIsUndefined(dest, src Register, line int) {
	c.emitOpCode(vm.OpIsUndefined, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(src))
}

// emitIsNullish emits OpIsNullish instruction: dest = (src === null || src === undefined)
func (c *Compiler) emitIsNullish(dest, src Register, line int) {
	c.emitOpCode(vm.OpIsNullish, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(src))
}

// emitJumpIfNull emits OpJumpIfNull instruction: jump to offset if src === null
func (c *Compiler) emitJumpIfNull(src Register, offset int16, line int) {
	c.emitOpCode(vm.OpJumpIfNull, line)
	c.emitByte(byte(src))
	c.emitUint16(uint16(offset))
}

// emitJumpIfUndefined emits OpJumpIfUndefined instruction: jump to offset if src === undefined
func (c *Compiler) emitJumpIfUndefined(src Register, offset int16, line int) {
	c.emitOpCode(vm.OpJumpIfUndefined, line)
	c.emitByte(byte(src))
	c.emitUint16(uint16(offset))
}

// emitJumpIfNullish emits OpJumpIfNullish instruction: jump to offset if src is null or undefined
func (c *Compiler) emitJumpIfNullish(src Register, offset int16, line int) {
	c.emitOpCode(vm.OpJumpIfNullish, line)
	c.emitByte(byte(src))
	c.emitUint16(uint16(offset))
}

// --- With Statement Support ---

// emitPushWithObject emits OpPushWithObject instruction
func (c *Compiler) emitPushWithObject(objReg Register, line int) {
	c.emitOpCode(vm.OpPushWithObject, line)
	c.emitByte(byte(objReg))
}

// emitPopWithObject emits OpPopWithObject instruction
func (c *Compiler) emitPopWithObject(line int) {
	c.emitOpCode(vm.OpPopWithObject, line)
}

// emitGetWithProperty emits OpGetWithProperty instruction
func (c *Compiler) emitGetWithProperty(destReg Register, nameIdx int, line int) {
	c.emitOpCode(vm.OpGetWithProperty, line)
	c.emitByte(byte(destReg))
	c.emitUint16(uint16(nameIdx))
}

// emitSetWithProperty emits OpSetWithProperty instruction
func (c *Compiler) emitSetWithProperty(nameIdx int, valueReg Register, line int) {
	c.emitOpCode(vm.OpSetWithProperty, line)
	c.emitUint16(uint16(nameIdx))
	c.emitByte(byte(valueReg))
}

// emitLoadUninitialized emits OpLoadUninitialized to mark a register as TDZ (Temporal Dead Zone)
// This is used for let/const variables that haven't been initialized yet.
func (c *Compiler) emitLoadUninitialized(dest Register, line int) {
	c.emitOpCode(vm.OpLoadUninitialized, line)
	c.emitByte(byte(dest))
}

// emitCheckUninitialized emits OpCheckUninitialized to check if a register contains
// the TDZ marker (uninitialized let/const). Throws ReferenceError if uninitialized.
// The opcode self-rewrites to OpNop on successful check for performance.
func (c *Compiler) emitCheckUninitialized(reg Register, line int) {
	c.emitOpCode(vm.OpCheckUninitialized, line)
	c.emitByte(byte(reg))
}

// emitCloseUpvalue emits OpCloseUpvalue to close any open upvalue pointing to reg.
// Used for per-iteration bindings in for loops - ensures closures created in
// different iterations capture different values.
func (c *Compiler) emitCloseUpvalue(reg Register, line int) {
	c.emitOpCode(vm.OpCloseUpvalue, line)
	c.emitByte(byte(reg))
}

// emitTDZError emits code to throw a ReferenceError for accessing a variable
// before it is initialized (Temporal Dead Zone violation in default parameters)
func (c *Compiler) emitTDZError(hint Register, varName string, line int) {
	// Allocate registers for function and result
	funcReg := c.regAlloc.Alloc()
	resultReg := c.regAlloc.Alloc()

	// Load ReferenceError constructor
	refErrorGlobalIdx := c.GetOrAssignGlobalIndex("ReferenceError")
	c.emitGetGlobal(funcReg, refErrorGlobalIdx, line)

	// Load error message - OpCall expects args at funcReg+1, so load directly there
	msg := fmt.Sprintf("Cannot access '%s' before initialization", varName)
	msgConstIdx := c.chunk.AddConstant(vm.String(msg))
	argReg := funcReg + 1
	c.emitLoadConstant(argReg, msgConstIdx, line)

	// Call ReferenceError constructor
	c.emitCall(resultReg, funcReg, 1, line)

	// Throw the error
	c.emitOpCode(vm.OpThrow, line)
	c.emitByte(byte(resultReg))

	// Free temporary registers
	c.regAlloc.Free(funcReg)
	c.regAlloc.Free(resultReg)
}

// emitConstAssignmentError emits code to throw a TypeError for assigning to a const variable.
// ECMAScript spec: Assignment to const is a TypeError.
func (c *Compiler) emitConstAssignmentError(varName string, line int) {
	// Allocate registers for function and result
	funcReg := c.regAlloc.Alloc()
	resultReg := c.regAlloc.Alloc()

	// Load TypeError constructor
	typeErrorGlobalIdx := c.GetOrAssignGlobalIndex("TypeError")
	c.emitGetGlobal(funcReg, typeErrorGlobalIdx, line)

	// Load error message - OpCall expects args at funcReg+1, so load directly there
	msg := fmt.Sprintf("Assignment to constant variable '%s'", varName)
	msgConstIdx := c.chunk.AddConstant(vm.String(msg))
	argReg := funcReg + 1
	c.emitLoadConstant(argReg, msgConstIdx, line)

	// Call TypeError constructor
	c.emitCall(resultReg, funcReg, 1, line)

	// Throw the error
	c.emitOpCode(vm.OpThrow, line)
	c.emitByte(byte(resultReg))

	// Free temporary registers
	c.regAlloc.Free(funcReg)
	c.regAlloc.Free(resultReg)
}

// --- END NEW ---

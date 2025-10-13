package compiler

import (
	"paserati/pkg/vm"
)

// --- Bytecode Emission Helpers ---

func (c *Compiler) emitOpCode(op vm.OpCode, line int) {
	c.chunk.WriteOpCode(op, line)
}

func (c *Compiler) emitByte(b byte) {
	c.chunk.WriteByte(b)
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
	c.emitOpCode(vm.OpMove, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(src))
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

// emitNew emits OpNew with constructor register and argument count
func (c *Compiler) emitNew(dest, constructorReg Register, argCount byte, line int) {
	c.emitOpCode(vm.OpNew, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(constructorReg))
	c.emitByte(argCount)
}

// emitSpreadNew emits OpSpreadNew for constructor calls with spread arguments
func (c *Compiler) emitSpreadNew(dest, constructorReg, spreadArgReg Register, line int) {
	c.emitOpCode(vm.OpSpreadNew, line)
	c.emitByte(byte(dest))
	c.emitByte(byte(constructorReg))
	c.emitByte(byte(spreadArgReg))
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

// Added helper for OpSetUpvalue
func (c *Compiler) emitSetUpvalue(upvalueIndex uint8, srcReg Register, line int) {
	c.emitOpCode(vm.OpSetUpvalue, line)
	c.emitByte(byte(upvalueIndex))
	c.emitByte(byte(srcReg))
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

// emitDefineMethod emits OpDefineMethod ObjReg, ValueReg, NameConstIdx(Uint16)
// Used for defining non-enumerable methods (e.g., class methods)
func (c *Compiler) emitDefineMethod(obj, val Register, nameConstIdx uint16, line int) {
	c.emitOpCode(vm.OpDefineMethod, line)
	c.emitByte(byte(obj))
	c.emitByte(byte(val))
	c.emitUint16(nameConstIdx)
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

// --- END NEW ---

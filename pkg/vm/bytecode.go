package vm

import (
	"fmt"
	// "paserati/pkg/value" // No longer needed
	//"paserati/pkg/value"
	"strings"
)

// OpCode defines the type for bytecode instructions.
type OpCode uint8

// Enum for Opcodes (Register Machine)
const (
	// Format: OpCode <DestReg> <Operand1> <Operand2> ...
	// Operands can be registers or immediate values (like constant indices)

	OpLoadConst     OpCode = 0 // Rx ConstIdx: Load constant Constants[ConstIdx] into register Rx.
	OpLoadNull      OpCode = 1 // Rx: Load null value into register Rx.
	OpLoadUndefined OpCode = 2 // Rx: Load undefined value into register Rx.
	OpLoadTrue      OpCode = 3 // Rx: Load boolean true into register Rx.
	OpLoadFalse     OpCode = 4 // Rx: Load boolean false into register Rx.
	OpMove          OpCode = 5 // Rx Ry: Move value from register Ry into register Rx.

	// Arithmetic (Dest, Left, Right)
	OpAdd      OpCode = 6 // Rx Ry Rz: Rx = Ry + Rz
	OpSubtract OpCode = 7 // Rx Ry Rz: Rx = Ry - Rz
	OpMultiply OpCode = 8 // Rx Ry Rz: Rx = Ry * Rz
	OpDivide   OpCode = 9 // Rx Ry Rz: Rx = Ry / Rz

	// Unary
	OpNegate OpCode = 10 // Rx Ry: Rx = -Ry
	OpNot    OpCode = 11 // Rx Ry: Rx = !Ry (logical not)

	// Comparison (Result Dest, Left, Right) -> Result is boolean
	OpEqual          OpCode = 12 // Rx Ry Rz: Rx = (Ry == Rz)
	OpNotEqual       OpCode = 13 // Rx Ry Rz: Rx = (Ry != Rz)
	OpStrictEqual    OpCode = 14 // Rx Ry Rz: Rx = (Ry === Rz)
	OpStrictNotEqual OpCode = 15 // Rx Ry Rz: Rx = (Ry !== Rz)
	OpGreater        OpCode = 16 // Rx Ry Rz: Rx = (Ry > Rz)
	OpLess           OpCode = 17 // Rx Ry Rz: Rx = (Ry < Rz)
	OpLessEqual      OpCode = 18 // Rx Ry Rz: Rx = (Ry <= Rz)
	// Add GreaterEqual later if needed

	// Function/Call related
	OpCall   OpCode = 19 // Rx FuncReg ArgCount: Call function in FuncReg with ArgCount args, result in Rx
	OpReturn OpCode = 20 // Rx: Return value from register Rx.

	// Closure related
	OpClosure         OpCode = 21 // Rx FuncConstIdx UpvalueCount [IsLocal1 Index1 IsLocal2 Index2 ...]: Create closure for function Const[FuncConstIdx] with UpvalueCount upvalues, store in Rx.
	OpLoadFree        OpCode = 22 // Rx UpvalueIndex: Load free variable (upvalue) at index UpvalueIndex into register Rx.
	OpSetUpvalue      OpCode = 23 // UpvalueIndex Ry: Store value from register Ry into upvalue at index UpvalueIndex.
	OpReturnUndefined OpCode = 24 // No operands: Return undefined value from current function.

	// Control Flow
	OpJumpIfFalse OpCode = 25 // Rx Offset(16bit): Jump by Offset if Rx is falsey.
	OpJump        OpCode = 26 // Offset(16bit): Unconditionally jump by Offset.

	// Array Operations (NEW)
	OpMakeArray OpCode = 27 // DestReg StartReg Count: Create array in DestReg from Count values starting at StartReg.
	OpGetIndex  OpCode = 28 // DestReg ArrayReg IndexReg: DestReg = ArrayReg[IndexReg]
	OpSetIndex  OpCode = 29 // ArrayReg IndexReg ValueReg: ArrayReg[IndexReg] = ValueReg

	OpGetLength OpCode = 30 // <<< NEW: DestReg SrcReg: DestReg = length(SrcReg)

	// Add comparison operators as needed
	// OpLessEqual // Rx Ry Rz: Rx = (Ry <= Rz)
)

// String returns a human-readable name for the OpCode.
func (op OpCode) String() string {
	switch op {
	case OpLoadConst:
		return "OpLoadConst"
	case OpLoadNull:
		return "OpLoadNull"
	case OpLoadUndefined:
		return "OpLoadUndefined"
	case OpLoadTrue:
		return "OpLoadTrue"
	case OpLoadFalse:
		return "OpLoadFalse"
	case OpMove:
		return "OpMove"
	case OpAdd:
		return "OpAdd"
	case OpSubtract:
		return "OpSubtract"
	case OpMultiply:
		return "OpMultiply"
	case OpDivide:
		return "OpDivide"
	case OpNegate:
		return "OpNegate"
	case OpNot:
		return "OpNot"
	case OpEqual:
		return "OpEqual"
	case OpNotEqual:
		return "OpNotEqual"
	case OpStrictEqual:
		return "OpStrictEqual"
	case OpStrictNotEqual:
		return "OpStrictNotEqual"
	case OpGreater:
		return "OpGreater"
	case OpLess:
		return "OpLess"
	case OpCall:
		return "OpCall"
	case OpReturn:
		return "OpReturn"
	case OpClosure:
		return "OpClosure"
	case OpLoadFree:
		return "OpLoadFree"
	case OpSetUpvalue:
		return "OpSetUpvalue"
	case OpReturnUndefined:
		return "OpReturnUndefined"
	case OpJumpIfFalse:
		return "OpJumpIfFalse"
	case OpJump:
		return "OpJump"
	case OpLessEqual:
		return "OpLessEqual"
	case OpMakeArray:
		return "OpMakeArray"
	case OpGetIndex:
		return "OpGetIndex"
	case OpSetIndex:
		return "OpSetIndex"
	case OpGetLength:
		return "OpGetLength"
	default:
		return fmt.Sprintf("UnknownOpcode(%d)", op)
	}
}

// Chunk represents a sequence of bytecode instructions and associated data.
type Chunk struct {
	Code      []byte  // The bytecode instructions (OpCodes and operands)
	Constants []Value // Constant pool (Now uses Value from vm package)
	Lines     []int   // Line number corresponding to the start of each instruction
	// Add MaxRegs later for function definitions
}

// GetLine returns the source line number corresponding to a given bytecode offset.
// It assumes the Lines slice is populated correctly (same length as Code, storing line per OpCode).
func (c *Chunk) GetLine(offset int) int {
	// Basic bounds check
	if offset < 0 || offset >= len(c.Lines) {
		// Return 0 or -1 to indicate an invalid offset or missing line info?
		// Let's return 0, assuming line numbers are 1-based.
		return 0
	}
	return c.Lines[offset]
}

// NewChunk creates a new, empty Chunk.
func NewChunk() *Chunk {
	return &Chunk{
		Code:      make([]byte, 0),
		Constants: make([]Value, 0),
		Lines:     make([]int, 0),
	}
}

// WriteOpCode adds an opcode to the chunk.
func (c *Chunk) WriteOpCode(op OpCode, line int) {
	c.Code = append(c.Code, byte(op))
	c.Lines = append(c.Lines, line)
}

// WriteByte adds a raw byte (operand) to the chunk.
// Note: Line number is not tracked per operand, only per opcode.
func (c *Chunk) WriteByte(b byte) {
	c.Code = append(c.Code, b)
}

// WriteUint16 adds a 16-bit unsigned integer operand (e.g., for larger constant indices or jump offsets).
// Encoded as Big Endian.
func (c *Chunk) WriteUint16(val uint16) {
	c.Code = append(c.Code, byte(val>>8))
	c.Code = append(c.Code, byte(val&0xff))
}

// AddConstant adds a value to the chunk's constant pool and returns its index.
// Returns a uint16 as we might need more than 256 constants.
func (c *Chunk) AddConstant(v Value) uint16 {
	// TODO: Check if constant already exists to avoid duplicates
	c.Constants = append(c.Constants, v)
	idx := len(c.Constants) - 1
	if idx > 65535 {
		// Handle error: Too many constants
		panic("Too many constants in one chunk.")
	}
	return uint16(idx)
}

// --- Disassembly ---

// DisassembleChunk returns a human-readable string representation of the chunk.
func (c *Chunk) DisassembleChunk(name string) string {
	var builder strings.Builder // Use strings.Builder for efficient concatenation
	builder.WriteString(fmt.Sprintf("== %s ==\n", name))
	offset := 0
	for offset < len(c.Code) {
		offset = c.disassembleInstruction(&builder, offset)
	}
	return builder.String()
}

// disassembleInstruction appends the string representation of a single instruction
// to the builder and returns the offset of the next instruction.
func (c *Chunk) disassembleInstruction(builder *strings.Builder, offset int) int {
	builder.WriteString(fmt.Sprintf("%04d      ", offset))

	instruction := OpCode(c.Code[offset])
	switch instruction {
	case OpLoadConst:
		return c.registerConstantInstruction(builder, instruction.String(), offset, true)
	case OpLoadNull, OpLoadUndefined, OpLoadTrue, OpLoadFalse, OpReturn:
		return c.registerInstruction(builder, instruction.String(), offset) // Rx
	case OpNegate, OpNot, OpMove:
		return c.registerRegisterInstruction(builder, instruction.String(), offset) // Rx, Ry
	case OpAdd, OpSubtract, OpMultiply, OpDivide, OpEqual, OpNotEqual, OpStrictEqual, OpStrictNotEqual, OpGreater, OpLess, OpLessEqual:
		return c.registerRegisterRegisterInstruction(builder, instruction.String(), offset) // Rx, Ry, Rz

	case OpCall:
		return c.callInstruction(builder, instruction.String(), offset) // Rx, FuncReg, ArgCount

	// Closure instructions
	case OpLoadFree:
		return c.registerByteInstruction(builder, instruction.String(), offset, "UpvalueIdx") // Rx, UpvalueIndex
	case OpSetUpvalue:
		return c.byteRegisterInstruction(builder, instruction.String(), offset, "UpvalueIdx") // UpvalueIndex, Ry
	case OpClosure:
		return c.closureInstruction(builder, instruction.String(), offset)

	case OpReturnUndefined:
		return c.simpleInstruction(builder, instruction.String(), offset)

	// Control Flow
	case OpJumpIfFalse:
		return c.jumpInstruction(builder, instruction.String(), offset, true) // Has register operand
	case OpJump:
		return c.jumpInstruction(builder, instruction.String(), offset, false) // No register operand

	// Array Operations
	case OpMakeArray:
		return c.makeArrayInstruction(builder, instruction.String(), offset)
	case OpGetIndex:
		return c.getIndexInstruction(builder, instruction.String(), offset)
	case OpSetIndex:
		return c.setIndexInstruction(builder, instruction.String(), offset)

	default:
		builder.WriteString(fmt.Sprintf("Unknown opcode %d\n", instruction))
		return offset + 1
	}
}

// registerInstruction handles disassembly of instructions like OpCode Rx
func (c *Chunk) registerInstruction(builder *strings.Builder, name string, offset int) int {
	if offset+1 >= len(c.Code) {
		builder.WriteString(fmt.Sprintf("%s (missing register operand)\n", name))
		return offset + 1
	}
	reg := c.Code[offset+1]
	builder.WriteString(fmt.Sprintf("%-16s R%d\n", name, reg))
	return offset + 2
}

// registerRegisterInstruction handles disassembly of instructions like OpCode Rx, Ry
func (c *Chunk) registerRegisterInstruction(builder *strings.Builder, name string, offset int) int {
	if offset+2 >= len(c.Code) {
		builder.WriteString(fmt.Sprintf("%s (missing register operands)\n", name))
		if offset+1 < len(c.Code) {
			return offset + 2
		}
		return offset + 1
	}
	regX := c.Code[offset+1]
	regY := c.Code[offset+2]
	builder.WriteString(fmt.Sprintf("%-16s R%d, R%d\n", name, regX, regY))
	return offset + 3
}

// registerRegisterRegisterInstruction handles disassembly of instructions like OpCode Rx, Ry, Rz
func (c *Chunk) registerRegisterRegisterInstruction(builder *strings.Builder, name string, offset int) int {
	if offset+3 >= len(c.Code) {
		builder.WriteString(fmt.Sprintf("%s (missing register operands)\n", name))
		if offset+2 < len(c.Code) {
			return offset + 3
		}
		if offset+1 < len(c.Code) {
			return offset + 2
		}
		return offset + 1
	}
	regX := c.Code[offset+1]
	regY := c.Code[offset+2]
	regZ := c.Code[offset+3]
	builder.WriteString(fmt.Sprintf("%-16s R%d, R%d, R%d\n", name, regX, regY, regZ))
	return offset + 4 // Opcode + 3 register bytes
}

// registerConstantInstruction handles OpCode Rx, ConstIdx
// If isUint16Const is true, reads a 2-byte constant index.
func (c *Chunk) registerConstantInstruction(builder *strings.Builder, name string, offset int, isUint16Const bool) int {
	operandSize := 1
	if isUint16Const {
		operandSize = 2
	}

	if offset+1+operandSize > len(c.Code) {
		builder.WriteString(fmt.Sprintf("%s (missing operands)\n", name))
		if offset+1 < len(c.Code) {
			return offset + 2
		}
		return offset + 1
	}

	reg := c.Code[offset+1]
	var constantIndex uint16
	if isUint16Const {
		constantIndex = uint16(c.Code[offset+2])<<8 | uint16(c.Code[offset+3])
	} else {
		constantIndex = uint16(c.Code[offset+2])
	}

	if int(constantIndex) >= len(c.Constants) {
		builder.WriteString(fmt.Sprintf("%-16s R%d, %d (invalid constant index)\n", name, reg, constantIndex))
	} else {
		constantValue := c.Constants[constantIndex]
		builder.WriteString(fmt.Sprintf("%-16s R%d, %d ('%v')\n", name, reg, constantIndex, constantValue))
	}
	return offset + 1 + 1 + operandSize
}

// --- New Disassembly Helpers ---

// callInstruction handles OpCall Rx, FuncReg, ArgCount
func (c *Chunk) callInstruction(builder *strings.Builder, name string, offset int) int {
	if offset+3 >= len(c.Code) {
		builder.WriteString(fmt.Sprintf("%s (missing operands)\n", name))
		if offset+2 < len(c.Code) {
			return offset + 3
		}
		if offset+1 < len(c.Code) {
			return offset + 2
		}
		return offset + 1
	}
	destReg := c.Code[offset+1]
	funcReg := c.Code[offset+2]
	argCount := c.Code[offset+3]
	builder.WriteString(fmt.Sprintf("%-16s R%d, R%d, Args %d\n", name, destReg, funcReg, argCount))
	return offset + 4 // Opcode + Rx + Ry + ArgCount
}

// registerByteInstruction handles OpCode Rx, ByteVal (e.g., OpLoadFree Rx, UpvalueIndex)
func (c *Chunk) registerByteInstruction(builder *strings.Builder, name string, offset int, operandName string) int {
	if offset+2 >= len(c.Code) {
		builder.WriteString(fmt.Sprintf("%s (missing operands)\n", name))
		if offset+1 < len(c.Code) {
			return offset + 2
		}
		return offset + 1
	}
	regX := c.Code[offset+1]
	byteVal := c.Code[offset+2]
	builder.WriteString(fmt.Sprintf("%-16s R%d, %s %d\n", name, regX, operandName, byteVal))
	return offset + 3
}

// byteRegisterInstruction handles OpCode ByteVal, Ry (e.g., OpSetUpvalue UpvalueIndex, Ry)
func (c *Chunk) byteRegisterInstruction(builder *strings.Builder, name string, offset int, operandName string) int {
	if offset+2 >= len(c.Code) {
		builder.WriteString(fmt.Sprintf("%s (missing operands)\n", name))
		if offset+1 < len(c.Code) {
			return offset + 2
		}
		return offset + 1
	}
	byteVal := c.Code[offset+1]
	regY := c.Code[offset+2]
	builder.WriteString(fmt.Sprintf("%-16s %s %d, R%d\n", name, operandName, byteVal, regY))
	return offset + 3
}

// closureInstruction handles OpClosure Rx FuncConstIdx UpvalueCount [IsLocal Idx ...]
func (c *Chunk) closureInstruction(builder *strings.Builder, name string, offset int) int {
	// Opcode + Rx + FuncConstIdx(2 bytes) + UpvalueCount(1 byte) = 5 bytes minimum
	if offset+4 >= len(c.Code) {
		builder.WriteString(fmt.Sprintf("%s (missing operands)\n", name))
		// Determine how many bytes we actually read before failing
		if offset+3 < len(c.Code) {
			return offset + 4
		}
		if offset+2 < len(c.Code) {
			return offset + 3
		}
		if offset+1 < len(c.Code) {
			return offset + 2
		}
		return offset + 1
	}

	destReg := c.Code[offset+1]
	funcConstIdx := uint16(c.Code[offset+2])<<8 | uint16(c.Code[offset+3])
	upvalueCount := int(c.Code[offset+4])

	// Check if function constant is valid
	funcProtoStr := "invalid const idx"
	if int(funcConstIdx) < len(c.Constants) {
		funcVal := c.Constants[funcConstIdx]
		if IsFunction(funcVal) { // Check type first (uses local IsFunction)
			fn := AsFunction(funcVal) // Use local AsFunction, returns *Function
			if fn != nil {
				funcProtoStr = fn.Name
				if funcProtoStr == "" {
					funcProtoStr = "<script>" // Or maybe just <fn>?
				}
			} else {
				// Should not happen if IsFunction is true and AsFunction doesn't panic
				funcProtoStr = "<nil func>"
			}
		} else {
			funcProtoStr = "not a function"
		}
	}

	builder.WriteString(fmt.Sprintf("%-16s R%d, FnConst %d (%s), Upvalues %d\n",
		name, destReg, funcConstIdx, funcProtoStr, upvalueCount))

	// Check if we have enough bytes for all upvalue pairs
	endOffset := offset + 5 + (upvalueCount * 2)
	if endOffset > len(c.Code) {
		builder.WriteString(fmt.Sprintf("  %04d      (missing upvalue data)\n", offset+5))
		return len(c.Code) // Consume rest of bytecode as invalid
	}

	// Print upvalue details
	currentOffset := offset + 5
	for i := 0; i < upvalueCount; i++ {
		isLocal := c.Code[currentOffset] == 1
		index := c.Code[currentOffset+1]
		location := "Upvalue"
		if isLocal {
			location = "LocalReg"
		}
		builder.WriteString(fmt.Sprintf("  %04d      | %-8s %d\n", currentOffset, location, index))
		currentOffset += 2
	}

	return currentOffset // Return offset of the next instruction
}

// simpleInstruction handles instructions with no operands.
func (c *Chunk) simpleInstruction(builder *strings.Builder, name string, offset int) int {
	builder.WriteString(fmt.Sprintf("%s\n", name))
	return offset + 1 // Only consume the opcode byte
}

// jumpInstruction handles disassembly of jump instructions.
func (c *Chunk) jumpInstruction(builder *strings.Builder, name string, offset int, hasRegister bool) int {
	operandOffset := 1
	if hasRegister {
		operandOffset = 2
	}

	// Need 2 bytes for the jump offset
	if offset+operandOffset+1 >= len(c.Code) {
		builder.WriteString(fmt.Sprintf("%s (missing jump offset)\n", name))
		// Approximate return offset
		if offset+operandOffset < len(c.Code) {
			return offset + operandOffset + 1
		}
		if offset+operandOffset-1 < len(c.Code) {
			return offset + operandOffset
		}
		return offset + 1
	}

	jumpOffset := int16(uint16(c.Code[offset+operandOffset])<<8 | uint16(c.Code[offset+operandOffset+1]))
	jumpTarget := offset + 3 + int(jumpOffset) // Offset relative to *after* this instruction
	if hasRegister {
		jumpTarget++ // Account for register byte
		reg := c.Code[offset+1]
		builder.WriteString(fmt.Sprintf("%-16s R%d, %d (to %04d)\n", name, reg, jumpOffset, jumpTarget))
		return offset + 4 // Op + Reg + Offset(2)
	} else {
		builder.WriteString(fmt.Sprintf("%-16s %d (to %04d)\n", name, jumpOffset, jumpTarget))
		return offset + 3 // Op + Offset(2)
	}
}

// makeArrayInstruction handles OpMakeArray DestReg StartReg Count
func (c *Chunk) makeArrayInstruction(builder *strings.Builder, name string, offset int) int {
	if offset+3 >= len(c.Code) {
		builder.WriteString(fmt.Sprintf("%s (missing operands)\n", name))
		if offset+2 < len(c.Code) {
			return offset + 3
		}
		if offset+1 < len(c.Code) {
			return offset + 2
		}
		return offset + 1
	}
	destReg := c.Code[offset+1]
	startReg := c.Code[offset+2]
	count := c.Code[offset+3]
	builder.WriteString(fmt.Sprintf("%-16s R%d, R%d, Count %d\n", name, destReg, startReg, count))
	return offset + 4 // Opcode + 3 register bytes
}

// getIndexInstruction handles OpGetIndex DestReg ArrayReg IndexReg
func (c *Chunk) getIndexInstruction(builder *strings.Builder, name string, offset int) int {
	if offset+3 >= len(c.Code) {
		builder.WriteString(fmt.Sprintf("%s (missing operands)\n", name))
		if offset+2 < len(c.Code) {
			return offset + 3
		}
		if offset+1 < len(c.Code) {
			return offset + 2
		}
		return offset + 1
	}
	destReg := c.Code[offset+1]
	arrayReg := c.Code[offset+2]
	indexReg := c.Code[offset+3]
	builder.WriteString(fmt.Sprintf("%-16s R%d, R%d, R%d\n", name, destReg, arrayReg, indexReg))
	return offset + 4 // Opcode + 3 register bytes
}

// setIndexInstruction handles OpSetIndex ArrayReg IndexReg ValueReg
func (c *Chunk) setIndexInstruction(builder *strings.Builder, name string, offset int) int {
	if offset+3 >= len(c.Code) {
		builder.WriteString(fmt.Sprintf("%s (missing operands)\n", name))
		if offset+2 < len(c.Code) {
			return offset + 3
		}
		if offset+1 < len(c.Code) {
			return offset + 2
		}
		return offset + 1
	}
	arrayReg := c.Code[offset+1]
	indexReg := c.Code[offset+2]
	valueReg := c.Code[offset+3]
	builder.WriteString(fmt.Sprintf("%-16s R%d, R%d, R%d\n", name, arrayReg, indexReg, valueReg))
	return offset + 4 // Opcode + 3 register bytes
}

package bytecode

import (
	"fmt"
	"paseratti2/pkg/value"
	"strings"
)

// OpCode defines the type for bytecode instructions.
type OpCode uint8

// Function represents a compiled function.
// Moved here from value package to break import cycle.
type Function struct {
	Arity        int    // Number of parameters expected
	Chunk        *Chunk // Bytecode for the function body
	Name         string // Optional name for debugging
	RegisterSize int    // Number of registers needed for this function's frame
	// TODO: Add UpvalueCount later for closures
}

// Enum for Opcodes (Register Machine)
const (
	// Format: OpCode <DestReg> <Operand1> <Operand2> ...
	// Operands can be registers or immediate values (like constant indices)

	OpLoadConst     OpCode = iota // Rx ConstIdx: Load constant Constants[ConstIdx] into register Rx.
	OpLoadNull                    // Rx: Load null value into register Rx.
	OpLoadUndefined               // Rx: Load undefined value into register Rx.
	OpLoadTrue                    // Rx: Load boolean true into register Rx.
	OpLoadFalse                   // Rx: Load boolean false into register Rx.
	OpMove                        // Rx Ry: Move value from register Ry into register Rx.

	// Arithmetic (Dest, Left, Right)
	OpAdd      // Rx Ry Rz: Rx = Ry + Rz
	OpSubtract // Rx Ry Rz: Rx = Ry - Rz
	OpMultiply // Rx Ry Rz: Rx = Ry * Rz
	OpDivide   // Rx Ry Rz: Rx = Ry / Rz

	// Unary
	OpNegate // Rx Ry: Rx = -Ry
	OpNot    // Rx Ry: Rx = !Ry (logical not)

	// Comparison (Result Dest, Left, Right) -> Result is boolean
	OpEqual    // Rx Ry Rz: Rx = (Ry == Rz)
	OpNotEqual // Rx Ry Rz: Rx = (Ry != Rz)
	OpGreater  // Rx Ry Rz: Rx = (Ry > Rz)
	OpLess     // Rx Ry Rz: Rx = (Ry < Rz)
	// Add GreaterEqual, LessEqual later if needed

	// Function/Call related
	OpCall   // Rx FuncReg ArgCount: Call function in FuncReg with ArgCount args, result in Rx
	OpReturn // Rx: Return value from register Rx.

	// Closure related
	OpClosure         // Rx FuncConstIdx UpvalueCount [IsLocal1 Index1 IsLocal2 Index2 ...]: Create closure for function Const[FuncConstIdx] with UpvalueCount upvalues, store in Rx.
	OpLoadFree        // Rx UpvalueIndex: Load free variable (upvalue) at index UpvalueIndex into register Rx.
	OpSetUpvalue      // UpvalueIndex Ry: Store value from register Ry into upvalue at index UpvalueIndex.
	OpReturnUndefined // No operands: Return undefined value from current function.

	// Control Flow
	OpJumpIfFalse // Rx Offset(16bit): Jump by Offset if Rx is falsey.
	OpJump        // Offset(16bit): Unconditionally jump by Offset.

	// Add comparison operators as needed
	OpLessEqual // Rx Ry Rz: Rx = (Ry <= Rz)
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
	default:
		return fmt.Sprintf("UnknownOpcode(%d)", op)
	}
}

// Chunk represents a sequence of bytecode instructions and associated data.
type Chunk struct {
	Code      []byte        // The bytecode instructions (OpCodes and operands)
	Constants []value.Value // Constant pool
	Lines     []int         // Line number corresponding to the start of each instruction
	// Add MaxRegs later for function definitions
}

// NewChunk creates a new, empty Chunk.
func NewChunk() *Chunk {
	return &Chunk{
		Code:      make([]byte, 0),
		Constants: make([]value.Value, 0),
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
func (c *Chunk) AddConstant(v value.Value) uint16 {
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
		offset = c.disassembleInstruction(&builder, offset, -1)
	}
	return builder.String()
}

// disassembleInstruction appends the string representation of a single instruction
// to the builder and returns the offset of the next instruction.
func (c *Chunk) disassembleInstruction(builder *strings.Builder, offset int, line int) int {
	builder.WriteString(fmt.Sprintf("%04d      ", offset))

	instruction := OpCode(c.Code[offset])
	switch instruction {
	case OpLoadConst:
		return c.registerConstantInstruction(builder, instruction.String(), offset, true)
	case OpLoadNull, OpLoadUndefined, OpLoadTrue, OpLoadFalse, OpReturn:
		return c.registerInstruction(builder, instruction.String(), offset) // Rx
	case OpNegate, OpNot, OpMove:
		return c.registerRegisterInstruction(builder, instruction.String(), offset) // Rx, Ry
	case OpAdd, OpSubtract, OpMultiply, OpDivide, OpEqual, OpNotEqual, OpGreater, OpLess, OpLessEqual:
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
		if value.IsFunction(funcVal) { // Check type first
			if f, ok := value.AsFunction(funcVal).(*Function); ok { // Use helper and assert
				funcProtoStr = f.Name
			} else {
				funcProtoStr = "invalid func type"
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

// // Old stack-based disassembly functions (kept for reference, will remove later)
// func (c *Chunk) constantInstruction(name string, offset int) int { ... }
// func simpleInstruction(name string, offset int) int { ... }

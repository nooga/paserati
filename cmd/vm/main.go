package main

import (
	"fmt"
	"os"

	"paserati/pkg/vm"
)

func main() {
	fmt.Println("--- Paserati VM [Register] --- (Test Mode)")

	// Create a VM instance
	myVM := vm.NewVM()

	// Manually create a simple bytecode chunk
	chunk := vm.NewChunk()

	// Add a constant value (e.g., the number 456)
	constantIndex := chunk.AddConstant(vm.Number(456))

	// --- Generate Register-Based Instructions ---
	const line = 1

	// Instruction: OpLoadConst R0, const_idx
	chunk.WriteOpCode(vm.OpLoadConst, line)
	chunk.WriteByte(0)               // Destination Register: R0
	chunk.WriteUint16(constantIndex) // Constant index (now 16-bit)

	// Instruction: OpReturn R0
	chunk.WriteOpCode(vm.OpReturn, line)
	chunk.WriteByte(0) // Return value from Register: R0

	// Disassemble the chunk for verification
	fmt.Println("--- Disassembled Chunk ---")
	chunk.DisassembleChunk("Test Chunk")
	fmt.Println("------------------------")

	// Interpret the chunk
	fmt.Println("--- VM Execution ---")
	result := myVM.Interpret(chunk)
	fmt.Printf("--- Result: %v ---\\n", result)

	if result != vm.InterpretOK {
		os.Exit(1) // Exit with error code if interpretation failed
	}
}

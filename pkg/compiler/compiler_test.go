package compiler

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"paseratti2/pkg/lexer"
	"paseratti2/pkg/parser"
	"paseratti2/pkg/vm"
	"reflect"
	"strings"
	"testing"
)

// Helper function to create expected instruction sequences
func makeInstructions(ops ...interface{}) []byte {
	var instructions []byte
	for _, op := range ops {
		switch v := op.(type) {
		case vm.OpCode:
			instructions = append(instructions, byte(v))
			// If opcode has no operands, continue to next op in loop
			switch v {
			case vm.OpReturnUndefined:
				continue // No operands follow
			}
		case Register:
			instructions = append(instructions, byte(v))
		case byte:
			instructions = append(instructions, v)
		case int:
			// Handle multi-byte operands if necessary later, for now assume byte
			if v < 0 || v > 255 {
				panic(fmt.Sprintf("Integer operand %d out of byte range", v))
			}
			instructions = append(instructions, byte(v))
		case uint16:
			instructions = append(instructions, byte(v>>8))   // High byte
			instructions = append(instructions, byte(v&0xff)) // Low byte
		default:
			panic(fmt.Sprintf("Unsupported operand type %T in makeInstructions", op))
		}
	}
	return instructions
}

func TestCompileSimpleVariables(t *testing.T) {
	input := `
        // test.ts (slightly modified for clarity)
        let x = 123.45;
        const y = "hello";
        let z = true;
        let a = x; // Read variable x
        return a;   // Return the value read from x
    `

	// Expected Bytecode based on observed output
	expectedConstants := []vm.Value{
		vm.Number(123.45),
		vm.String("hello"),
	}
	expectedInstructions := makeInstructions(
		// let x = 123.45; (Value -> R0, Define x = R0)
		vm.OpLoadConst, Register(0), uint16(0), // R0 = Constants[0] (123.45)
		// const y = "hello"; (Value -> R1, Define y = R1)
		vm.OpLoadConst, Register(1), uint16(1), // R1 = Constants[1] ("hello")
		// let z = true; (Value -> R2, Define z = R2)
		vm.OpLoadTrue, Register(2),
		// let a = x; (Resolve x -> R0, OpMove R3, R0, Define a = R3)
		vm.OpMove, Register(3), Register(0), // R3 = R0
		// return a; (Resolve a -> R3, OpMove R4, R3)
		vm.OpMove, Register(4), Register(3), // R4 = R3
		// (Return statement uses last expression register R4)
		vm.OpReturn, Register(4),
		// Implicit final return uses OpReturnUndefined now
		vm.OpReturnUndefined,
	)

	// --- Run Compiler ---
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		t.Fatalf("Parser encountered errors:\n%v", p.Errors())
	}

	comp := NewCompiler()
	chunk, compilerErrors := comp.Compile(program)
	if len(compilerErrors) != 0 {
		t.Fatalf("Compiler encountered errors:\n%v", compilerErrors)
	}

	// --- Assertions ---

	// Compare instructions
	if !reflect.DeepEqual(chunk.Code, expectedInstructions) {
		t.Errorf("Instruction mismatch:")
		t.Errorf("  Input:    %q", input)
		t.Errorf("  Expected: %v", expectedInstructions)
		t.Errorf("  Got:      %v", chunk.Code)
		t.Errorf("--- Disassembled Expected (approx) ---")
		t.Logf("\n%s", printOpCodesToString(expectedInstructions))
		t.Errorf("--- Disassembled Got ---")
		t.Logf("\n%s", chunk.DisassembleChunk("Compiled Chunk"))
		t.FailNow()
	}

	// Compare constants
	if !reflect.DeepEqual(chunk.Constants, expectedConstants) {
		t.Errorf("Constant pool mismatch:")
		t.Errorf("  Expected: %v", expectedConstants)
		t.Errorf("  Got:      %v", chunk.Constants)
		t.FailNow()
	}
}

func TestCompileExpressions(t *testing.T) {
	tests := []struct {
		input                string
		expectedConstants    []vm.Value
		expectedInstructions []byte
	}{
		{
			input:             "-5;",
			expectedConstants: []vm.Value{vm.Number(5)},
			expectedInstructions: makeInstructions(
				vm.OpLoadConst, Register(0), uint16(0),
				vm.OpNegate, Register(1), Register(0),
				vm.OpReturn, Register(1),
			),
		},
		{
			input:             "!true;",
			expectedConstants: []vm.Value{},
			expectedInstructions: makeInstructions(
				vm.OpLoadTrue, Register(0),
				vm.OpNot, Register(1), Register(0),
				vm.OpReturn, Register(1),
			),
		},
		{
			input: "5 + 10 * 2 - 1 / 1;",
			// Constants: 5, 10, 2, 1, 1 (no deduplication yet)
			expectedConstants: []vm.Value{vm.Number(5), vm.Number(10), vm.Number(2), vm.Number(1), vm.Number(1)},
			expectedInstructions: makeInstructions(
				vm.OpLoadConst, Register(0), uint16(0), // R0 = 5
				vm.OpLoadConst, Register(1), uint16(1), // R1 = 10
				vm.OpLoadConst, Register(2), uint16(2), // R2 = 2
				vm.OpMultiply, Register(3), Register(1), Register(2), // R3 = R1 * R2 (20)
				vm.OpAdd, Register(4), Register(0), Register(3), // R4 = R0 + R3 (25)
				vm.OpLoadConst, Register(5), uint16(3), // R5 = 1 (const index 3)
				vm.OpLoadConst, Register(6), uint16(4), // R6 = 1 (const index 4 - NO DEDUPE)
				vm.OpDivide, Register(7), Register(5), Register(6), // R7 = R5 / R6 (1)
				vm.OpSubtract, Register(8), Register(4), Register(7), // R8 = R4 - R7 (24)
				vm.OpReturn, Register(8),
			),
		},
		{
			input:             "let a = 5; let b = 10; a < b;",
			expectedConstants: []vm.Value{vm.Number(5), vm.Number(10)},
			expectedInstructions: makeInstructions(
				vm.OpLoadConst, Register(0), uint16(0),
				vm.OpLoadConst, Register(1), uint16(1),
				vm.OpMove, Register(2), Register(0),
				vm.OpMove, Register(3), Register(1),
				vm.OpLess, Register(4), Register(2), Register(3),
				vm.OpReturn, Register(4),
			),
		},
		{
			input: "(5 + 5) * 2 == 20;",
			// Constants: 5, 5, 2, 20 (no deduplication yet)
			expectedConstants: []vm.Value{vm.Number(5), vm.Number(5), vm.Number(2), vm.Number(20)},
			expectedInstructions: makeInstructions(
				vm.OpLoadConst, Register(0), uint16(0), // R0 = 5 (index 0)
				vm.OpLoadConst, Register(1), uint16(1), // R1 = 5 (index 1 - NO DEDUPE)
				vm.OpAdd, Register(2), Register(0), Register(1), // R2 = R0 + R1 (10)
				vm.OpLoadConst, Register(3), uint16(2), // R3 = 2 (index 2)
				vm.OpMultiply, Register(4), Register(2), Register(3), // R4 = R2 * R3 (20)
				vm.OpLoadConst, Register(5), uint16(3), // R5 = 20 (index 3)
				vm.OpEqual, Register(6), Register(4), Register(5), // R6 = R4 == R5 (true)
				vm.OpReturn, Register(6),
			),
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("Input_%d", i), func(t *testing.T) {
			l := lexer.NewLexer(tt.input)
			p := parser.NewParser(l)
			program := p.ParseProgram()
			if len(p.Errors()) != 0 {
				t.Fatalf("Parser errors:\n%v", p.Errors())
			}

			comp := NewCompiler()
			chunk, compilerErrors := comp.Compile(program)
			if len(compilerErrors) != 0 {
				t.Fatalf("Compiler errors:\n%v", compilerErrors)
			}

			// Compare instructions
			if !reflect.DeepEqual(chunk.Code, tt.expectedInstructions) {
				t.Errorf("Instruction mismatch:")
				t.Errorf("  Input:    %q", tt.input)
				t.Errorf("  Expected: %v", tt.expectedInstructions)
				t.Errorf("  Got:      %v", chunk.Code)
				t.Errorf("--- Disassembled Expected (approx) ---")
				t.Logf("\n%s", printOpCodesToString(tt.expectedInstructions))
				t.Errorf("--- Disassembled Got ---")
				t.Logf("\n%s", chunk.DisassembleChunk("Compiled Chunk"))
				t.FailNow()
			}

			// Compare constants
			if !reflect.DeepEqual(chunk.Constants, tt.expectedConstants) {
				t.Errorf("Constant pool mismatch:")
				t.Errorf("  Input:    %q", tt.input)
				t.Errorf("  Expected: %v", tt.expectedConstants)
				t.Errorf("  Got:      %v", chunk.Constants)
				t.FailNow()
			}
		})
	}
}

func TestCompileFunctions(t *testing.T) {
	input := `
        let double = function(x) { return x * 2; };
        let result = double(10);
        return result;
    `

	// Expected Constants for the MAIN chunk:
	// 0: Function object for 'double'
	// 1: Number 10 (for the argument)
	expectedMainConstants := []vm.Value{
		vm.NewFunction(&vm.Function{}), // Placeholder for Function check
		vm.Number(10),
	}

	// Expected Instructions for the 'double' FUNCTION chunk:
	// Parameters: x (R0)
	expectedFuncInstructions := makeInstructions(
		// x * 2:
		vm.OpMove, Register(1), Register(0), // R1 = R0 (load x)
		vm.OpLoadConst, Register(2), uint16(0), // R2 = 2 (Constant index 0 within func chunk)
		vm.OpMultiply, Register(3), Register(1), Register(2), // R3 = R1 * R2
		// return R3:
		vm.OpReturn, Register(3),
		// Implicit return added by compiler:
		vm.OpReturnUndefined,
	)
	expectedFuncConstants := []vm.Value{vm.Number(2)}

	// Expected Instructions for the MAIN chunk (Adjusted Registers):
	expectedMainInstructions := makeInstructions(
		// let double = function(x) { ... };
		// Compiles FuncLit -> creates closure -> R0, Define double = R0
		vm.OpClosure, Register(0), uint16(0), byte(0), // R0 = Closure(FuncConst=0, Upvalues=0)

		// let result = double(10);
		// Compile double -> R1 (resolve R0, move R0->R1)
		vm.OpMove, Register(1), Register(0),
		// Compile 10 -> R2
		vm.OpLoadConst, Register(2), uint16(1), // R2 = 10 (Const Idx 1)
		// Emit Call R3, R1, 1 (Result in R3, Func/Closure in R1, ArgCount 1)
		vm.OpCall, Register(3), Register(1), byte(1),
		// Define result = R3

		// return result;
		// Compile result -> R4 (resolve R3, move R3->R4)
		vm.OpMove, Register(4), Register(3),
		// Emit return R4
		vm.OpReturn, Register(4),

		// Implicit final return uses OpReturnUndefined now
		vm.OpReturnUndefined,
	)

	// --- Run Compiler ---
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		t.Fatalf("Parser errors:\n%v", p.Errors())
	}

	comp := NewCompiler()
	mainChunk, compilerErrors := comp.Compile(program)
	if len(compilerErrors) != 0 {
		t.Fatalf("Compiler errors:\n%v", compilerErrors)
	}

	// --- Assertions ---

	// 1. Check Main Chunk Instructions
	if !reflect.DeepEqual(mainChunk.Code, expectedMainInstructions) {
		t.Errorf("Main instruction mismatch:")
		t.Errorf("  Expected: %v", expectedMainInstructions)
		t.Errorf("  Got:      %v", mainChunk.Code)
		t.Errorf("--- Disassembled Expected Main (approx) ---")
		t.Logf("\n%s", printOpCodesToString(expectedMainInstructions))
		t.Errorf("--- Disassembled Got Main ---")
		t.Logf("\n%s", mainChunk.DisassembleChunk("Main Chunk"))
		t.FailNow()
	}

	// 2. Check number of constants in Main Chunk
	if len(mainChunk.Constants) != len(expectedMainConstants) {
		t.Fatalf("Main constant count mismatch: expected %d, got %d (Got: %v)",
			len(expectedMainConstants), len(mainChunk.Constants), mainChunk.Constants)
	}

	// 3. Check non-function constants in Main Chunk
	if !reflect.DeepEqual(mainChunk.Constants[1], expectedMainConstants[1]) {
		t.Errorf("Main constant mismatch (non-function):")
		t.Errorf("  Expected Constant[1]: %v", expectedMainConstants[1])
		t.Errorf("  Got Constant[1]:      %v", mainChunk.Constants[1])
		t.FailNow()
	}

	// 4. Check the Function constant in Main Chunk
	funcVal := mainChunk.Constants[0]
	if !vm.IsFunction(funcVal) {
		t.Fatalf("Main constant[0] is not a function: got %T (%v)", funcVal, funcVal)
	}
	compiledFunc := vm.AsFunction(funcVal)

	// 5. Check Function Properties (Arity, Name)
	expectedArity := 1
	expectedName := "double"
	// TODO: Check RegSize? Hard to predict exactly with simple allocator.
	if compiledFunc.Arity != expectedArity {
		t.Errorf("Function arity mismatch: expected %d, got %d", expectedArity, compiledFunc.Arity)
	}
	if compiledFunc.Name != expectedName {
		t.Errorf("Function name mismatch: expected %q, got %q", expectedName, compiledFunc.Name)
	}

	// 6. Check Function Chunk Instructions
	if !reflect.DeepEqual(compiledFunc.Chunk.Code, expectedFuncInstructions) {
		t.Errorf("Function instruction mismatch (%s):", compiledFunc.Name)
		t.Errorf("  Expected: %v", expectedFuncInstructions)
		t.Errorf("  Got:      %v", compiledFunc.Chunk.Code)
		t.Errorf("--- Disassembled Expected Func (approx) ---")
		t.Logf("\n%s", printOpCodesToString(expectedFuncInstructions))
		t.Errorf("--- Disassembled Got Func ---")
		t.Logf("\n%s", compiledFunc.Chunk.DisassembleChunk(compiledFunc.Name))
		t.FailNow()
	}

	// 7. Check Function Chunk Constants
	if !reflect.DeepEqual(compiledFunc.Chunk.Constants, expectedFuncConstants) {
		t.Errorf("Function constant pool mismatch (%s):", compiledFunc.Name)
		t.Errorf("  Expected: %v", expectedFuncConstants)
		t.Errorf("  Got:      %v", compiledFunc.Chunk.Constants)
		t.FailNow()
	}
}

// logAllChunks recursively disassembles and logs a chunk and any function chunks within its constants.
func logAllChunks(t *testing.T, chunk *vm.Chunk, name string, logged map[interface{}]bool) {
	if chunk == nil {
		return
	}
	if logged[chunk] { // Avoid infinite loops with recursive constants
		return
	}
	logged[chunk] = true

	t.Logf("--- Disassembly [%s] ---\n%s", name, chunk.DisassembleChunk(name))

	for i, constant := range chunk.Constants {
		var funcProto *vm.Function
		constName := fmt.Sprintf("%s Const[%d]", name, i)

		if vm.IsFunction(constant) {
			funcProto = vm.AsFunction(constant)
		} else if vm.IsClosure(constant) {
			closure := vm.AsClosure(constant)
			funcProto = closure.Fn
		}

		if funcProto != nil {
			// Use function name if available for better logging
			funcChunkName := constName
			if funcProto.Name != "" {
				funcChunkName = fmt.Sprintf("%s (%s)", constName, funcProto.Name)
			}
			logAllChunks(t, funcProto.Chunk, funcChunkName, logged) // Recurse
		}
	}
}

func TestClosures(t *testing.T) {
	input := `
let makeAdder = function(x) { // Outer function
  let adder = function(y) { // Inner function (closure)
    return x + y; // Captures 'x'
  };
  return adder;
};

let add5 = makeAdder(5);
let result = add5(10); // Call the closure
return result; // Explicitly return the result
`
	compiler := NewCompiler()
	program, errs := compileSource(input)
	if len(errs) > 0 {
		t.Fatalf("Parser errors: %v", errs)
	}

	chunk, compileErrs := compiler.Compile(program)
	if len(compileErrs) > 0 {
		t.Errorf("Compiler errors: %v", compileErrs)
		logAllChunks(t, chunk, "Closure Test Compile Error", make(map[interface{}]bool))
		t.FailNow()
	}

	// Redirect Stdout to capture VM output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	vmInstance := vm.NewVM()
	interpretResult := vmInstance.Interpret(chunk)

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	vmOutput := strings.TrimSpace(buf.String())

	if interpretResult != vm.InterpretOK {
		t.Errorf("VM execution failed with result: %v", interpretResult)
		logAllChunks(t, chunk, "Closure Test Runtime Error", make(map[interface{}]bool))
		t.FailNow()
	}

	expectedOutput := "15" // VM should print the final result
	if vmOutput != expectedOutput {
		t.Errorf("VM output mismatch.\nExpected: %q\nGot:      %q", expectedOutput, vmOutput)
		logAllChunks(t, chunk, "Closure Test Mismatch", make(map[interface{}]bool))
	}
}

func TestValuesNullUndefined(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Basic Values
		{"return null;", "null"},
		{"return;", "undefined"},          // Implicit return is undefined (Parser fix)
		{"let x; return x;", "undefined"}, // Uninitialized variable (Parser/Compiler fix)

		// Equality (Strict)
		{"return null == null;", "true"},
		// {"return undefined == undefined;", "true"}, // Cannot use 'undefined' keyword
		{"return null == false;", "false"},
		{"return null == true;", "false"},
		{"return null == 0;", "false"},
		{"return null == \"\";", "false"},
		// {"return undefined == false;", "false"},  // Cannot use 'undefined' keyword
		// {"return undefined == 0;", "false"},     // Cannot use 'undefined' keyword

		// Inequality (Strict)
		{"return null != null;", "false"},
		// {"return undefined != undefined;", "false"}, // Cannot use 'undefined' keyword
		// {"return null != undefined;", "true"},    // Cannot use 'undefined' keyword
		{"return 0 != null;", "true"},
		{"return false != null;", "true"},

		// Truthiness
		{"return !null;", "true"},
		// {"return !undefined;", "true"},          // Cannot use 'undefined' keyword
		{"return !false;", "true"},
		{"return !true;", "false"},
		{"return !0;", "false"}, // 0 is considered truthy in this simple check
		{"return !1;", "false"},
		{"return !\"\";", "false"}, // Empty string is truthy
		{"return !\"a\";", "false"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			compiler := NewCompiler()
			program, errs := compileSource(tt.input)
			if len(errs) > 0 {
				t.Fatalf("Parser errors: %v", errs)
			}

			chunk, compileErrs := compiler.Compile(program)
			if len(compileErrs) > 0 {
				t.Errorf("Compiler errors: %v", compileErrs)
				logAllChunks(t, chunk, "Value Test Compile Error", make(map[interface{}]bool))
				t.FailNow()
			}

			// Capture VM output
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			vmInstance := vm.NewVM()
			interpretResult := vmInstance.Interpret(chunk)

			w.Close()
			os.Stdout = oldStdout

			var buf bytes.Buffer
			io.Copy(&buf, r)
			vmOutput := strings.TrimSpace(buf.String())

			if interpretResult != vm.InterpretOK {
				t.Errorf("VM execution failed with result: %v", interpretResult)
				logAllChunks(t, chunk, "Value Test Runtime Error", make(map[interface{}]bool))
				t.FailNow()
			}

			if vmOutput != tt.expected {
				t.Errorf("VM output mismatch.\nInput:    %q\nExpected: %q\nGot:      %q", tt.input, tt.expected, vmOutput)
				logAllChunks(t, chunk, "Value Test Mismatch", make(map[interface{}]bool))
			}
		})
	}
}

func TestRecursion(t *testing.T) {
	input := `
let countdown = function(n) {
  if (n <= 0) {
    return 0;
  }
  countdown(n - 1);
  return n; // Return n so final result is from initial call
};

countdown(3); // Should eventually return 3
`
	compiler := NewCompiler()
	program, errs := compileSource(input)
	if len(errs) > 0 {
		t.Fatalf("Parser errors: %v", errs)
	}

	chunk, compileErrs := compiler.Compile(program)
	if len(compileErrs) > 0 {
		t.Errorf("Compiler errors: %v", compileErrs)
		logAllChunks(t, chunk, "Recursion Test Compile Error", make(map[interface{}]bool))
		t.FailNow()
	}

	// Capture VM output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	vmInstance := vm.NewVM()
	interpretResult := vmInstance.Interpret(chunk)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	vmOutput := strings.TrimSpace(buf.String())

	if interpretResult != vm.InterpretOK {
		t.Errorf("VM execution failed with result: %v", interpretResult)
		logAllChunks(t, chunk, "Recursion Test Runtime Error", make(map[interface{}]bool))
		t.FailNow()
	}

	expectedOutput := "3" // Final result of countdown(3)
	if vmOutput != expectedOutput {
		t.Errorf("VM output mismatch.\nInput:    %q\nExpected: %q\nGot:      %q", input, expectedOutput, vmOutput)
		logAllChunks(t, chunk, "Recursion Test Mismatch", make(map[interface{}]bool))
	}
}

// printOpCodesToString - Basic helper to disassemble expected bytes to string
func printOpCodesToString(code []byte) string {
	var builder strings.Builder
	offset := 0
	for offset < len(code) {
		opCodeByte := code[offset]
		op := vm.OpCode(opCodeByte)
		builder.WriteString(fmt.Sprintf("%04d %-16s", offset, op))

		length := 1 // Assume 1 for unknown
		switch op {
		case vm.OpLoadConst:
			length = 4 // Op + Reg + Const(2)
		case vm.OpLoadNull, vm.OpLoadTrue, vm.OpLoadFalse, vm.OpReturn:
			length = 2 // Op + Reg
		case vm.OpNegate, vm.OpNot, vm.OpMove:
			length = 3 // Op + Dest + Src
		case vm.OpAdd, vm.OpSubtract, vm.OpMultiply, vm.OpDivide,
			vm.OpEqual, vm.OpNotEqual, vm.OpGreater, vm.OpLess,
			vm.OpCall:
			length = 4 // Op + Dest + Left/Func + Right/ArgCount
		case vm.OpReturnUndefined:
			length = 1 // Just the opcode
		default:
			builder.WriteString(" (Unknown Op)")
			length = 1 // Default guess
		}
		builder.WriteString(fmt.Sprintf(" (len %d)\n", length))

		// Avoid index out of bounds if instruction is partial/malformed
		if offset+length > len(code) {
			// Optionally print remaining bytes
			builder.WriteString(fmt.Sprintf("  WARN: Instruction bytes truncated? Remaining: %v\n", code[offset:]))
			break
		}

		// Basic operand printing (can enhance later)
		if length > 1 {
			builder.WriteString(fmt.Sprintf("        Operands: %v\n", code[offset+1:offset+length]))
		}

		offset += length
	}
	return builder.String()
}

// compileSource is a helper to lex and parse input code for tests.
func compileSource(input string) (*parser.Program, []string) {
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	program := p.ParseProgram()
	if len(p.Errors()) != 0 {
		return nil, p.Errors()
	}
	return program, nil
}

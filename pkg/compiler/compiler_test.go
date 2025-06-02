package compiler

import (
	"fmt"
	// "os"
	// "bytes"
	// "io"

	"paserati/pkg/vm"
	"reflect"
	"strings"
	"testing"

	// "paserati/pkg/driver" // REMOVED - Causes import cycle
	// Keep
	"paserati/pkg/errors"
	"paserati/pkg/lexer"
	"paserati/pkg/parser"
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

func _TestCompileSimpleVariables(t *testing.T) {
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

	// --- Parse ---
	program, parseErrs := compileSource(input) // Use helper
	if len(parseErrs) > 0 {
		var errMsgs strings.Builder
		// errors.DisplayErrors(input, parseErrs)
		for _, e := range parseErrs {
			errMsgs.WriteString(e.Error() + "\n")
		}
		t.Fatalf("Parser encountered errors:\n%s", errMsgs.String())
	}
	if program == nil {
		t.Fatalf("Parser returned nil program without errors")
	}

	// --- Compile ---
	comp := NewCompiler()
	chunk, compileErrs := comp.Compile(program)
	if len(compileErrs) > 0 {
		var errMsgs strings.Builder
		// errors.DisplayErrors(input, compileErrs)
		for _, e := range compileErrs {
			errMsgs.WriteString(e.Error() + "\n")
		}
		t.Fatalf("Compiler encountered errors:\n%s", errMsgs.String())
	}
	if chunk == nil {
		t.Fatalf("Compiler returned nil chunk without errors")
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

// compareConstants compares two slices of vm.Value, handling string values properly
func compareConstants(t *testing.T, got, expected []vm.Value, inputDesc string) {
	if len(got) != len(expected) {
		t.Errorf("Constant count mismatch for %s:", inputDesc)
		t.Errorf("  Expected: %d constants", len(expected))
		t.Errorf("  Got:      %d constants", len(got))
		t.FailNow()
	}

	for i := 0; i < len(expected); i++ {
		expectedVal := expected[i]
		gotVal := got[i]

		// Handle string constants specially
		if vm.IsString(expectedVal) && vm.IsString(gotVal) {
			if vm.AsString(expectedVal) != vm.AsString(gotVal) {
				t.Errorf("Constant mismatch at index %d for %s:", i, inputDesc)
				t.Errorf("  Expected: %q", vm.AsString(expectedVal))
				t.Errorf("  Got:      %q", vm.AsString(gotVal))
				t.FailNow()
			}
		} else {
			// For non-string constants, use reflect.DeepEqual
			if !reflect.DeepEqual(gotVal, expectedVal) {
				t.Errorf("Constant mismatch at index %d for %s:", i, inputDesc)
				t.Errorf("  Expected: %v", expectedVal)
				t.Errorf("  Got:      %v", gotVal)
				t.FailNow()
			}
		}
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
				vm.OpAdd, Register(2), Register(0), Register(3), // R4 = R0 + R3 (25)
				vm.OpLoadConst, Register(3), uint16(3), // R5 = 1 (const index 3)
				vm.OpLoadConst, Register(0), uint16(4), // R6 = 1 (const index 4 - NO DEDUPE)
				vm.OpDivide, Register(1), Register(3), Register(0), // R7 = R5 / R6 (1)
				vm.OpSubtract, Register(0), Register(2), Register(1), // R8 = R4 - R7 (24)
				vm.OpReturn, Register(0),
			),
		},
		{
			input:             "let a = 5; let b = 10; a < b;",
			expectedConstants: []vm.Value{vm.Number(5), vm.Number(10)}, // No name constants needed anymore
			expectedInstructions: makeInstructions(
				vm.OpLoadConst, Register(0), uint16(0), // R0 = 5
				vm.OpSetGlobal, uint16(0), Register(0), // Global[0] = R0 (first global variable)
				vm.OpLoadConst, Register(1), uint16(1), // R1 = 10
				vm.OpSetGlobal, uint16(1), Register(1), // Global[1] = R1 (second global variable)
				vm.OpGetGlobal, Register(2), uint16(0), // R2 = Global[0] (value of a)
				vm.OpGetGlobal, Register(3), uint16(1), // R3 = Global[1] (value of b)
				vm.OpLess, Register(4), Register(2), Register(3), // R4 = R2 < R3
				vm.OpReturn, Register(4), // Return R4
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
				vm.OpLoadConst, Register(1), uint16(2), // R3 = 2 (index 2)
				vm.OpMultiply, Register(0), Register(2), Register(1), // R4 = R2 * R3 (20)
				vm.OpLoadConst, Register(1), uint16(3), // R5 = 20 (index 3)
				vm.OpEqual, Register(2), Register(0), Register(1), // R6 = R4 == R5 (true)
				vm.OpReturn, Register(2),
			),
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("Input_%d", i), func(t *testing.T) {
			// --- Parse ---
			program, parseErrs := compileSource(tt.input) // Use helper
			if len(parseErrs) > 0 {
				var errMsgs strings.Builder
				// errors.DisplayErrors(tt.input, parseErrs)
				for _, e := range parseErrs {
					errMsgs.WriteString(e.Error() + "\n")
				}
				t.Fatalf("Parser errors:\n%s", errMsgs.String())
			}
			if program == nil {
				t.Fatalf("Parser returned nil program without errors")
			}

			// --- Compile ---
			comp := NewCompiler()
			chunk, compileErrs := comp.Compile(program)
			if len(compileErrs) > 0 {
				var errMsgs strings.Builder
				// errors.DisplayErrors(tt.input, compileErrs)
				for _, e := range compileErrs {
					errMsgs.WriteString(e.Error() + "\n")
				}
				t.Fatalf("Compiler errors:\n%s", errMsgs.String())
			}
			if chunk == nil {
				t.Fatalf("Compiler returned nil chunk without errors")
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
			compareConstants(t, chunk.Constants, tt.expectedConstants, fmt.Sprintf("Input_%d", i))
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
		vm.NewFunction(1, 0, 4, false, "double", vm.NewChunk()), // Placeholder for Function check
		vm.Number(10), // Function argument
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

	// Expected Instructions for the MAIN chunk (Global Variables):
	expectedMainInstructions := makeInstructions(
		// let double = function(x) { ... };
		// OPTIMIZATION: Functions with 0 upvalues use OpLoadConst instead of OpClosure
		vm.OpLoadConst, Register(0), uint16(0), // R0 = Function(FuncConst=0) - OPTIMIZED
		vm.OpSetGlobal, uint16(0), Register(0), // Global[0] = R0 (first global variable)

		// let result = double(10);
		// Compile double -> R1 (load global)
		vm.OpGetGlobal, Register(1), uint16(0), // R1 = Global[0] (double function)
		// Compile 10 -> R2
		vm.OpLoadConst, Register(2), uint16(1), // R2 = 10 (Const Idx 1)
		// Emit Call R3, R1, 1 (Result in R3, Func in R1, ArgCount 1)
		vm.OpCall, Register(3), Register(1), byte(1), // R3 = call double(R2)
		vm.OpSetGlobal, uint16(1), Register(3), // Global[1] = R3 (second global variable)

		// return result;
		// Compile result -> R4 (resolve global)
		vm.OpGetGlobal, Register(4), uint16(1), // R4 = Global[1] (result value)
		// Emit return R4
		vm.OpReturn, Register(4),
		// Extra OpReturn from final return logic
		vm.OpReturn, Register(4),
	)

	// --- Parse ---
	program, parseErrs := compileSource(input) // Updated call
	if len(parseErrs) > 0 {
		var errMsgs strings.Builder
		// errors.DisplayErrors(input, parseErrs)
		for _, e := range parseErrs {
			errMsgs.WriteString(e.Error() + "\n")
		}
		t.Fatalf("Parser errors:\n%s", errMsgs.String())
	}
	if program == nil {
		t.Fatalf("Parser returned nil program without errors")
	}

	// --- Compile ---
	comp := NewCompiler()
	mainChunk, compileErrs := comp.Compile(program) // Use updated return type
	if len(compileErrs) > 0 {
		var errMsgs strings.Builder
		// errors.DisplayErrors(input, compileErrs)
		for _, e := range compileErrs {
			errMsgs.WriteString(e.Error() + "\n")
		}
		t.Fatalf("Compiler errors:\n%s", errMsgs.String())
	}
	if mainChunk == nil {
		t.Fatalf("Compiler returned nil chunk without errors")
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
		t.Errorf("Main constant mismatch (number 10):")
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
		var funcProto *vm.FunctionObject
		constName := fmt.Sprintf("%s Const[%d]", name, i)

		if vm.IsFunction(constant) {
			funcProto = vm.AsFunction(constant)
		} else if constant.IsClosure() {
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
	// --- Parse ---
	program, parseErrs := compileSource(input)
	if len(parseErrs) > 0 {
		var errMsgs strings.Builder
		// errors.DisplayErrors(input, parseErrs)
		for _, e := range parseErrs {
			errMsgs.WriteString(e.Error() + "\n")
		}
		t.Fatalf("Parser errors:\n%s", errMsgs.String())
	}
	if program == nil {
		t.Fatalf("Parser returned nil program without errors")
	}

	// --- Compile ---
	compiler := NewCompiler()
	chunk, compileErrs := compiler.Compile(program)
	if len(compileErrs) > 0 {
		var errMsgs strings.Builder
		// errors.DisplayErrors(input, compileErrs)
		for _, e := range compileErrs {
			errMsgs.WriteString(e.Error() + "\n")
		}
		logAllChunks(t, chunk, "Closure Test Compile Error", make(map[interface{}]bool)) // chunk might be nil here
		t.Fatalf("Compiler errors:\n%s", errMsgs.String())
	}
	if chunk == nil {
		t.Fatalf("Compiler returned nil chunk without errors")
	}

	// --- Run VM ---
	vmInstance := vm.NewVM()
	finalValue, runtimeErrs := vmInstance.Interpret(chunk) // Updated call

	// --- Check VM Result ---
	if len(runtimeErrs) > 0 {
		var errMsgs strings.Builder
		// errors.DisplayErrors(input, runtimeErrs)
		for _, e := range runtimeErrs {
			errMsgs.WriteString(e.Error() + "\n")
		}
		logAllChunks(t, chunk, "Closure Test Runtime Error", make(map[interface{}]bool))
		t.Fatalf("VM execution failed:\n%s", errMsgs.String())
	}

	expectedOutput := "15"                // Expect the final value, not stdout
	actualOutput := finalValue.ToString() // Get string representation of the final value
	if actualOutput != expectedOutput {
		t.Errorf("VM result mismatch.\nExpected: %q\nGot:      %q", expectedOutput, actualOutput)
		logAllChunks(t, chunk, "Closure Test Mismatch", make(map[interface{}]bool))
	}
}

func TestValuesNullUndefined(t *testing.T) {
	tests := []struct {
		input    string
		expected string // Expected final Value as string
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
		{"return !0;", "true"}, // 0 is considered falsey
		{"return !1;", "false"},
		{"return !\"\";", "true"}, // Empty string is falsey
		{"return !\"a\";", "false"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			// --- Parse ---
			program, parseErrs := compileSource(tt.input)
			if len(parseErrs) > 0 {
				var errMsgs strings.Builder
				// errors.DisplayErrors(tt.input, parseErrs)
				for _, e := range parseErrs {
					errMsgs.WriteString(e.Error() + "\n")
				}
				t.Fatalf("Parser errors:\n%s", errMsgs.String())
			}
			if program == nil {
				t.Fatalf("Parser returned nil program without errors")
			}

			// --- Compile ---
			compiler := NewCompiler()
			chunk, compileErrs := compiler.Compile(program)
			if len(compileErrs) > 0 {
				var errMsgs strings.Builder
				// errors.DisplayErrors(tt.input, compileErrs)
				for _, e := range compileErrs {
					errMsgs.WriteString(e.Error() + "\n")
				}
				logAllChunks(t, chunk, "Value Test Compile Error", make(map[interface{}]bool))
				t.Fatalf("Compiler errors:\n%s", errMsgs.String())
			}
			if chunk == nil {
				t.Fatalf("Compiler returned nil chunk without errors")
			}

			// --- Run VM ---
			vmInstance := vm.NewVM()
			finalValue, runtimeErrs := vmInstance.Interpret(chunk) // Updated call

			// --- Check Result ---
			if len(runtimeErrs) > 0 {
				var errMsgs strings.Builder
				// errors.DisplayErrors(tt.input, runtimeErrs)
				for _, e := range runtimeErrs {
					errMsgs.WriteString(e.Error() + "\n")
				}
				logAllChunks(t, chunk, "Value Test Runtime Error", make(map[interface{}]bool))
				t.Fatalf("VM execution failed:\n%s", errMsgs.String())
			}

			actualOutput := finalValue.ToString()
			if actualOutput != tt.expected {
				t.Errorf("VM result mismatch.\nInput:    %q\nExpected: %q\nGot:      %q", tt.input, tt.expected, actualOutput)
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

// Explicitly return the result of the call for testing
return countdown(3);
`
	// --- Parse ---
	program, parseErrs := compileSource(input)
	if len(parseErrs) > 0 {
		var errMsgs strings.Builder
		// errors.DisplayErrors(input, parseErrs)
		for _, e := range parseErrs {
			errMsgs.WriteString(e.Error() + "\n")
		}
		t.Fatalf("Parser errors:\n%s", errMsgs.String())
	}
	if program == nil {
		t.Fatalf("Parser returned nil program without errors")
	}

	// --- Compile ---
	compiler := NewCompiler()
	chunk, compileErrs := compiler.Compile(program)
	if len(compileErrs) > 0 {
		var errMsgs strings.Builder
		// errors.DisplayErrors(input, compileErrs)
		for _, e := range compileErrs {
			errMsgs.WriteString(e.Error() + "\n")
		}
		logAllChunks(t, chunk, "Recursion Test Compile Error", make(map[interface{}]bool))
		t.Fatalf("Compiler errors:\n%s", errMsgs.String())
	}
	if chunk == nil {
		t.Fatalf("Compiler returned nil chunk without errors")
	}

	// --- Run VM ---
	vmInstance := vm.NewVM()
	finalValue, runtimeErrs := vmInstance.Interpret(chunk) // Updated call

	// --- Check Result ---
	if len(runtimeErrs) > 0 {
		var errMsgs strings.Builder
		// errors.DisplayErrors(input, runtimeErrs)
		for _, e := range runtimeErrs {
			errMsgs.WriteString(e.Error() + "\n")
		}
		logAllChunks(t, chunk, "Recursion Test Runtime Error", make(map[interface{}]bool))
		t.Fatalf("VM execution failed:\n%s", errMsgs.String())
	}

	expectedOutput := "3" // Final result of countdown(3)
	actualOutput := finalValue.ToString()
	if actualOutput != expectedOutput {
		t.Errorf("VM result mismatch.\nInput:    %q\nExpected: %q\nGot:      %q", input, expectedOutput, actualOutput)
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
		case vm.OpGetGlobal, vm.OpSetGlobal:
			length = 4 // Op + NameIdx(2) + Reg for OpGetGlobal, Op + NameIdx(2) + Reg for OpSetGlobal
		case vm.OpClosure: // Added OpClosure case
			if offset+3 >= len(code) { // Need at least Op(1) + Dst(1) + FuncIdx(2)
				builder.WriteString(" (WARN: Truncated Closure Op - Min Header)")
				length = len(code) - offset // Consume rest
			} else {
				numUpvalues := int(code[offset+3])
				expectedLen := 4 + numUpvalues*2 // Op + Dst + FuncIdx(2) + UpvalCount + (IsLocal+Idx)*N
				if offset+expectedLen > len(code) {
					builder.WriteString(fmt.Sprintf(" (WARN: Truncated Closure Op - Expected %d, Got %d bytes)", expectedLen, len(code)-offset))
					length = len(code) - offset // Consume rest
				} else {
					length = expectedLen
				}
			}
		case vm.OpReturnUndefined:
			length = 1 // Just the opcode
		default:
			builder.WriteString(" (Unknown Op)")
			length = 1 // Default guess
		}
		builder.WriteString(fmt.Sprintf(" (len %d)\\n", length))

		// Avoid index out of bounds if instruction is partial/malformed
		if offset+length > len(code) {
			// Optionally print remaining bytes
			builder.WriteString(fmt.Sprintf("  WARN: Instruction bytes truncated? Remaining: %v\\n", code[offset:]))
			break
		}

		// Basic operand printing (can enhance later)
		if length > 1 {
			builder.WriteString(fmt.Sprintf("        Operands: %v\\n", code[offset+1:offset+length]))
		}

		offset += length
	}
	return builder.String()
}

// compileSource is a helper to lex and parse input code for tests.
// Retained ONLY for TestCompileFunctions which needs the AST.
// Prefer driver.CompileString for most tests.
func compileSource(input string) (*parser.Program, []errors.PaseratiError) { // Updated return type
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	program, parseErrs := p.ParseProgram() // Updated call
	// Return program even if there are errors, caller should check parseErrs
	return program, parseErrs // Return program and errors
}

func TestCompoundAssignments(t *testing.T) {
	tests := []struct {
		name                 string
		input                string
		expectedConstants    []vm.Value
		expectedInstructions []byte
	}{
		{
			name:              "Add Assign Local",
			input:             "let x = 5; x += 3; x;",
			expectedConstants: []vm.Value{vm.Number(5), vm.Number(3)}, // No name constants
			expectedInstructions: makeInstructions(
				vm.OpLoadConst, Register(0), uint16(0), // R0 = 5
				vm.OpSetGlobal, uint16(0), Register(0), // Global[0] = R0
				vm.OpGetGlobal, Register(1), uint16(0), // R1 = Global[0] (current value of x)
				vm.OpLoadConst, Register(2), uint16(1), // R2 = 3
				vm.OpAdd, Register(3), Register(1), Register(2), // R3 = R1 + R2 (x = x + 3)
				vm.OpSetGlobal, uint16(0), Register(3), // Global[0] = R3 (store result back)
				vm.OpGetGlobal, Register(2), uint16(0), // R2 = Global[0] (load x for final expression)
				vm.OpReturn, Register(2), // Return R2
			),
		},
		{
			name:              "Subtract Assign Local",
			input:             "let y = 10; y -= 4; y;",
			expectedConstants: []vm.Value{vm.Number(10), vm.Number(4)}, // No name constants
			expectedInstructions: makeInstructions(
				vm.OpLoadConst, Register(0), uint16(0), // R0 = 10
				vm.OpSetGlobal, uint16(0), Register(0), // Global[0] = R0
				vm.OpGetGlobal, Register(1), uint16(0), // R1 = Global[0] (current value of y)
				vm.OpLoadConst, Register(2), uint16(1), // R2 = 4
				vm.OpSubtract, Register(3), Register(1), Register(2), // R3 = R1 - R2
				vm.OpSetGlobal, uint16(0), Register(3), // Global[0] = R3 (store result back)
				vm.OpGetGlobal, Register(2), uint16(0), // R2 = Global[0] (load y for final expression)
				vm.OpReturn, Register(2), // Return R2
			),
		},
		{
			name:              "Multiply Assign Local",
			input:             "let z = 2; z *= 6; z;",
			expectedConstants: []vm.Value{vm.Number(2), vm.Number(6)}, // No name constants
			expectedInstructions: makeInstructions(
				vm.OpLoadConst, Register(0), uint16(0), // R0 = 2
				vm.OpSetGlobal, uint16(0), Register(0), // Global[0] = R0
				vm.OpGetGlobal, Register(1), uint16(0), // R1 = Global[0] (current value of z)
				vm.OpLoadConst, Register(2), uint16(1), // R2 = 6
				vm.OpMultiply, Register(3), Register(1), Register(2), // R3 = R1 * R2
				vm.OpSetGlobal, uint16(0), Register(3), // Global[0] = R3 (store result back)
				vm.OpGetGlobal, Register(2), uint16(0), // R2 = Global[0] (load z for final expression)
				vm.OpReturn, Register(2), // Return R2
			),
		},
		{
			name:              "Divide Assign Local",
			input:             "let w = 12; w /= 3; w;",
			expectedConstants: []vm.Value{vm.Number(12), vm.Number(3)}, // No name constants
			expectedInstructions: makeInstructions(
				vm.OpLoadConst, Register(0), uint16(0), // R0 = 12
				vm.OpSetGlobal, uint16(0), Register(0), // Global[0] = R0
				vm.OpGetGlobal, Register(1), uint16(0), // R1 = Global[0] (current value of w)
				vm.OpLoadConst, Register(2), uint16(1), // R2 = 3
				vm.OpDivide, Register(3), Register(1), Register(2), // R3 = R1 / R2
				vm.OpSetGlobal, uint16(0), Register(3), // Global[0] = R3 (store result back)
				vm.OpGetGlobal, Register(2), uint16(0), // R2 = Global[0] (load w for final expression)
				vm.OpReturn, Register(2), // Return R2
			),
		},
		// TODO: Add tests for compound assignment with upvalues later?
	}

	// --- Test Runner Logic ---
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// --- Parse ---
			program, parseErrs := compileSource(tt.input)
			if len(parseErrs) > 0 {
				var errMsgs strings.Builder
				// errors.DisplayErrors(tt.input, parseErrs)
				for _, e := range parseErrs {
					errMsgs.WriteString(e.Error() + "\n")
				}
				t.Fatalf("Parser errors:\n%s", errMsgs.String())
			}
			if program == nil {
				t.Fatalf("Parser returned nil program without errors")
			}

			// --- Compile ---
			comp := NewCompiler()
			chunk, compileErrs := comp.Compile(program)
			if len(compileErrs) > 0 {
				var errMsgs strings.Builder
				// errors.DisplayErrors(tt.input, compileErrs)
				for _, e := range compileErrs {
					errMsgs.WriteString(e.Error() + "\n")
				}
				t.Fatalf("Compiler errors:\n%s", errMsgs.String())
			}
			if chunk == nil {
				t.Fatalf("Compiler returned nil chunk without errors")
			}

			// Compare instructions
			if !reflect.DeepEqual(chunk.Code, tt.expectedInstructions) {
				t.Errorf("Instruction mismatch for test '%s':", tt.name)
				t.Errorf("  Input:    %q", tt.input)
				t.Errorf("  Expected: %v", tt.expectedInstructions)
				t.Errorf("  Got:      %v", chunk.Code)
				t.Errorf("--- Disassembled Expected (approx) ---")
				t.Logf("\n%s", printOpCodesToString(tt.expectedInstructions))
				t.Errorf("--- Disassembled Got ---")
				t.Logf("\n%s", chunk.DisassembleChunk("Compiled Chunk - "+tt.name))
				t.FailNow()
			}

			// Compare constants
			compareConstants(t, chunk.Constants, tt.expectedConstants, tt.name)
		})
	}
}

func TestUpdateExpressions(t *testing.T) {
	tests := []struct {
		name                 string
		input                string
		expectedConstants    []vm.Value
		expectedInstructions []byte
	}{
		{
			name:              "Prefix Increment Local",
			input:             "let x = 5; let y = ++x; y;",           // x becomes 6, y is 6
			expectedConstants: []vm.Value{vm.Number(5), vm.Number(1)}, // No name constants
			expectedInstructions: makeInstructions(
				vm.OpLoadConst, Register(0), uint16(0), // R0 = 5
				vm.OpSetGlobal, uint16(0), Register(0), // Global[0] = R0 (x = 5)
				vm.OpGetGlobal, Register(1), uint16(0), // R1 = Global[0] (current value of x)
				vm.OpLoadConst, Register(2), uint16(1), // R2 = 1
				vm.OpAdd, Register(1), Register(1), Register(2), // R1 = R1 + R2 (x increments)
				vm.OpSetGlobal, uint16(0), Register(1), // Global[0] = R1 (store incremented x)
				vm.OpMove, Register(3), Register(1), // R3 = R1 (result of ++x is new value)
				vm.OpSetGlobal, uint16(1), Register(3), // Global[1] = R3 (y = incremented x)
				vm.OpGetGlobal, Register(1), uint16(1), // R1 = Global[1] (load y for final expression)
				vm.OpReturn, Register(1),
			),
		},
		{
			name:              "Postfix Increment Local",
			input:             "let x = 5; let y = x++; y;",           // x becomes 6, y is 5
			expectedConstants: []vm.Value{vm.Number(5), vm.Number(1)}, // No name constants
			expectedInstructions: makeInstructions(
				vm.OpLoadConst, Register(0), uint16(0), // R0 = 5
				vm.OpSetGlobal, uint16(0), Register(0), // Global[0] = R0 (x = 5)
				vm.OpGetGlobal, Register(1), uint16(0), // R1 = Global[0] (current value of x)
				vm.OpLoadConst, Register(2), uint16(1), // R2 = 1
				vm.OpMove, Register(3), Register(1), // R3 = R1 (save original value of x)
				vm.OpAdd, Register(1), Register(1), Register(2), // R1 = R1 + R2 (x increments)
				vm.OpSetGlobal, uint16(0), Register(1), // Global[0] = R1 (store incremented x)
				vm.OpSetGlobal, uint16(1), Register(3), // Global[1] = R3 (y = original value)
				vm.OpGetGlobal, Register(1), uint16(1), // R1 = Global[1] (load y for final expression)
				vm.OpReturn, Register(1),
			),
		},
		{
			name:              "Prefix Decrement Local",
			input:             "let x = 5; let y = --x; y;",           // x becomes 4, y is 4
			expectedConstants: []vm.Value{vm.Number(5), vm.Number(1)}, // No name constants
			expectedInstructions: makeInstructions(
				vm.OpLoadConst, Register(0), uint16(0), // R0 = 5
				vm.OpSetGlobal, uint16(0), Register(0), // Global[0] = R0 (x = 5)
				vm.OpGetGlobal, Register(1), uint16(0), // R1 = Global[0] (current value of x)
				vm.OpLoadConst, Register(2), uint16(1), // R2 = 1
				vm.OpSubtract, Register(1), Register(1), Register(2), // R1 = R1 - R2 (x decrements)
				vm.OpSetGlobal, uint16(0), Register(1), // Global[0] = R1 (store decremented x)
				vm.OpMove, Register(3), Register(1), // R3 = R1 (result of --x is new value)
				vm.OpSetGlobal, uint16(1), Register(3), // Global[1] = R3 (y = decremented x)
				vm.OpGetGlobal, Register(1), uint16(1), // R1 = Global[1] (load y for final expression)
				vm.OpReturn, Register(1),
			),
		},
		{
			name:              "Postfix Decrement Local",
			input:             "let x = 5; let y = x--; y;",           // x becomes 4, y is 5
			expectedConstants: []vm.Value{vm.Number(5), vm.Number(1)}, // No name constants
			expectedInstructions: makeInstructions(
				vm.OpLoadConst, Register(0), uint16(0), // R0 = 5
				vm.OpSetGlobal, uint16(0), Register(0), // Global[0] = R0 (x = 5)
				vm.OpGetGlobal, Register(1), uint16(0), // R1 = Global[0] (current value of x)
				vm.OpLoadConst, Register(2), uint16(1), // R2 = 1
				vm.OpMove, Register(3), Register(1), // R3 = R1 (save original value of x)
				vm.OpSubtract, Register(1), Register(1), Register(2), // R1 = R1 - R2 (x decrements)
				vm.OpSetGlobal, uint16(0), Register(1), // Global[0] = R1 (store decremented x)
				vm.OpSetGlobal, uint16(1), Register(3), // Global[1] = R3 (y = original value)
				vm.OpGetGlobal, Register(1), uint16(1), // R1 = Global[1] (load y for final expression)
				vm.OpReturn, Register(1),
			),
		},
		// TODO: Add tests for update expression with upvalues later?
	}

	// --- Test Runner Logic ---
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// --- Parse ---
			program, parseErrs := compileSource(tt.input)
			if len(parseErrs) > 0 {
				var errMsgs strings.Builder
				// errors.DisplayErrors(tt.input, parseErrs)
				for _, e := range parseErrs {
					errMsgs.WriteString(e.Error() + "\n")
				}
				t.Fatalf("Parser errors:\n%s", errMsgs.String())
			}
			if program == nil {
				t.Fatalf("Parser returned nil program without errors")
			}

			// --- Compile ---
			comp := NewCompiler()
			chunk, compileErrs := comp.Compile(program)
			if len(compileErrs) > 0 {
				var errMsgs strings.Builder
				// errors.DisplayErrors(tt.input, compileErrs)
				for _, e := range compileErrs {
					errMsgs.WriteString(e.Error() + "\n")
				}
				t.Fatalf("Compiler errors:\n%s", errMsgs.String())
			}
			if chunk == nil {
				t.Fatalf("Compiler returned nil chunk without errors")
			}

			// Compare instructions
			if !reflect.DeepEqual(chunk.Code, tt.expectedInstructions) {
				t.Errorf("Instruction mismatch for test '%s':", tt.name)
				t.Errorf("  Input:    %q", tt.input)
				t.Errorf("  Expected: %v", tt.expectedInstructions)
				t.Errorf("  Got:      %v", chunk.Code)
				t.Errorf("--- Disassembled Expected (approx) ---")
				t.Logf("\n%s", printOpCodesToString(tt.expectedInstructions))
				t.Errorf("--- Disassembled Got ---")
				t.Logf("\n%s", chunk.DisassembleChunk("Compiled Chunk - "+tt.name))
				t.FailNow()
			}

			// Compare constants
			compareConstants(t, chunk.Constants, tt.expectedConstants, tt.name)
		})
	}
}

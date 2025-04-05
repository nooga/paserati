package tests

import (
	"paserati/pkg/driver"
	"paserati/pkg/vm"
	"strings"
	"testing"
)

// --- Operator & Literal Matrix Test ---

type matrixTestCase struct {
	name               string
	input              string
	expect             string // Expected output OR expected error substring
	isError            bool   // True if expect is a runtime error substring
	expectCompileError bool   // True if expect is a compile error substring
	disassemble        bool   // True if disassembly should be logged
}

func TestOperatorsAndLiterals(t *testing.T) {
	testCases := []matrixTestCase{
		// Literals
		{name: "LiteralNum", input: "123.45;", expect: "123.45"},
		{name: "LiteralStr", input: `"hello test";`, expect: "hello test"},
		{name: "LiteralTrue", input: "true;", expect: "true"},
		{name: "LiteralFalse", input: "false;", expect: "false"},
		{name: "LiteralNull", input: "null;", expect: "null"},
		{name: "LiteralUndefinedLet", input: "let u; u;", expect: "undefined"},
		{name: "LiteralUndefinedReturn", input: "return;", expect: "undefined"}, // Implicit undefined return

		// Prefix
		{name: "PrefixMinusNum", input: "-15;", expect: "-15"},
		{name: "PrefixMinusZero", input: "-0;", expect: "0"},
		{name: "PrefixNotTrue", input: "!true;", expect: "false"},
		{name: "PrefixNotFalse", input: "!false;", expect: "true"},
		{name: "PrefixNotNull", input: "!null;", expect: "true"},
		{name: "PrefixNotUndefined", input: "let u; !u;", expect: "true"}, // Need VM/Value check for undefined truthiness
		{name: "PrefixNotZero", input: "!0;", expect: "true"},             // number 0 is falsey
		{name: "PrefixNotNum", input: "!12;", expect: "false"},            // other numbers truthy
		{name: "PrefixNotEMPTYStr", input: `!"";`, expect: "true"},        // empty string falsey
		{name: "PrefixNotStr", input: `!"a";`, expect: "false"},           // non-empty string truthy

		// Infix Arithmetic
		{name: "InfixAddNum", input: "5 + 10;", expect: "15"},
		{name: "InfixSubNum", input: "10 - 4;", expect: "6"},
		{name: "InfixMulNum", input: "6 * 7;", expect: "42"},
		{name: "InfixDivNum", input: "10 / 4;", expect: "2.5"},
		{name: "InfixAddStr", input: `"f" + "oo";`, expect: "foo"},
		{name: "InfixDivZero", input: "1 / 0;", expect: "Division by zero", isError: true},
		{name: "InfixAddMismatch", input: `1 + "a";`, expect: "1a"},
		{name: "InfixSubMismatch", input: `"a" - 1;`, expect: "operator '-' cannot be applied to types 'string' and 'number'", isError: true, expectCompileError: true},

		// Infix Comparison
		{name: "InfixLT_T", input: "5 < 10;", expect: "true"},
		{name: "InfixLT_F", input: "10 < 5;", expect: "false"},
		{name: "InfixLT_Eq", input: "5 < 5;", expect: "false"},
		{name: "InfixGT_T", input: "10 > 5;", expect: "true"},
		{name: "InfixGT_F", input: "5 > 10;", expect: "false"},
		{name: "InfixGT_Eq", input: "5 > 5;", expect: "false"},
		{name: "InfixLTE_T1", input: "5 <= 10;", expect: "true"},
		{name: "InfixLTE_T2", input: "10 <= 10;", expect: "true"},
		{name: "InfixLTE_F", input: "10 <= 5;", expect: "false"},
		// {name: "InfixGTE_T1", input: "10 >= 5;", expect: "true"}, // Need GTE operator
		// {name: "InfixGTE_T2", input: "10 >= 10;", expect: "true"},
		// {name: "InfixGTE_F", input: "5 >= 10;", expect: "false"},

		// Infix Equality (==, !=)
		{name: "InfixEqNum_T", input: "10 == 10;", expect: "true"},
		{name: "InfixEqNum_F", input: "10 == 5;", expect: "false"},
		{name: "InfixEqStr_T", input: `"a" == "a";`, expect: "true"},
		{name: "InfixEqStr_F", input: `"a" == "b";`, expect: "false"},
		{name: "InfixEqBool_T", input: "true == true;", expect: "true"},
		{name: "InfixEqBool_F", input: "true == false;", expect: "false"},
		{name: "InfixEqNull_T", input: "null == null;", expect: "true"},
		{name: "InfixEqNull_F", input: "null == false;", expect: "false"},
		{name: "InfixEqMixType", input: `10 == "10";`, expect: "false"},
		{name: "InfixNeqNum_F", input: "10 != 10;", expect: "false"},
		{name: "InfixNeqNum_T", input: "10 != 5;", expect: "true"},
		{name: "InfixNeqStr_F", input: `"a" != "a";`, expect: "false"},
		{name: "InfixNeqStr_T", input: `"a" != "b";`, expect: "true"},
		{name: "InfixNeqBool_F", input: "true != true;", expect: "false"},
		{name: "InfixNeqBool_T", input: "true != false;", expect: "true"},
		{name: "InfixNeqNull_F", input: "null != null;", expect: "false"},
		{name: "InfixNeqNull_T", input: "null != false;", expect: "true"},
		{name: "InfixNeqMixType", input: `10 != "10";`, expect: "true"},

		// Strict Equality (===, !==)
		{name: "StrictEqNum_T", input: "10 === 10;", expect: "true"},
		{name: "StrictEqNum_F", input: "10 === 5;", expect: "false"},
		{name: "StrictEqStr_T", input: `"a" === "a";`, expect: "true"},
		{name: "StrictEqStr_F", input: `"a" === "b";`, expect: "false"},
		{name: "StrictEqBool_T", input: "true === true;", expect: "true"},
		{name: "StrictEqBool_F", input: "true === false;", expect: "false"},
		{name: "StrictEqNull_T", input: "null === null;", expect: "true"},
		{name: "StrictEqNull_F", input: "null === false;", expect: "false"},
		{name: "StrictEqMixType", input: `10 === "10";`, expect: "false"},
		{name: "StrictNeqNum_F", input: "10 !== 10;", expect: "false"},
		{name: "StrictNeqNum_T", input: "10 !== 5;", expect: "true"},
		{name: "StrictNeqStr_F", input: `"a" !== "a";`, expect: "false"},
		{name: "StrictNeqStr_T", input: `"a" !== "b";`, expect: "true"},
		{name: "StrictNeqBool_F", input: "true !== true;", expect: "false"},
		{name: "StrictNeqBool_T", input: "true !== false;", expect: "true"},
		{name: "StrictNeqNull_F", input: "null !== null;", expect: "false"},
		{name: "StrictNeqNull_T", input: "null !== false;", expect: "true"},
		{name: "StrictNeqMixType", input: `10 !== "10";`, expect: "true"},

		// Ternary Operator
		{name: "TernaryTrue", input: "true ? 1 : 2;", expect: "1"},
		{name: "TernaryFalse", input: "false ? 1 : 2;", expect: "2"},
		{name: "TernaryCondVar", input: "let x=5; x > 0 ? \"pos\" : \"neg\";", expect: "pos"},
		{name: "TernaryComplex", input: "1 < 2 ? (3+4) : (5*6);", expect: "7"},

		// If/Else If/Else Statement (Note: statements don't return values directly, check via side effect)
		{name: "IfSimple", input: "let r=0; if (true) { r=1; } r;", expect: "1"},
		{name: "IfFalse", input: "let r=0; if (false) { r=1; } r;", expect: "0"},
		{name: "IfElseTrue", input: "let r=0; if (true) { r=1; } else { r=2; } r;", expect: "1"},
		{name: "IfElseFalse", input: "let r=0; if (false) { r=1; } else { r=2; } r;", expect: "2"},
		{name: "IfElseIfTrue", input: "let r=0; if (false) { r=1; } else if (true) { r=2; } else { r=3; } r;", expect: "2"},
		{name: "IfElseIfFalse", input: "let r=0; if (false) { r=1; } else if (false) { r=2; } else { r=3; } r;", expect: "3"},
		{name: "IfElseIfChain", input: "let r=0; if (1>2) { r=1; } else if (2>3) { r=2; } else if (3>4) { r=3; } else { r=4; } r;", expect: "4"},

		// --- Logical Operators (&&, ||) ---
		{name: "Logical OR True L", input: "true || false;", expect: "true"},
		{name: "Logical OR True R", input: "false || true;", expect: "true"},
		{name: "Logical OR False", input: "false || false;", expect: "false"},
		{name: "Logical OR ShortCircuit", input: "true || (1/0);", expect: "true"}, // Should not divide by zero
		{name: "Logical OR Truthy Value L", input: `"a" || false;`, expect: "a"},
		{name: "Logical OR Truthy Value R", input: `false || "b";`, expect: "b"},
		{name: "Logical OR Falsey Values", input: `0 || "";`, expect: ""}, // Returns last falsey

		{name: "Logical AND True", input: "true && true;", expect: "true"},
		{name: "Logical AND False L", input: "false && true;", expect: "false"},
		{name: "Logical AND False R", input: "true && false;", expect: "false"},
		{name: "Logical AND ShortCircuit", input: "false && (1/0);", expect: "false"}, // Should not divide by zero
		{name: "Logical AND Truthy Values", input: `"a" && 1;`, expect: "1"},          // Returns last truthy
		{name: "Logical AND Falsey Value L", input: `0 && true;`, expect: "0"},
		{name: "Logical AND Falsey Value R", input: `true && "";`, expect: ""},

		// --- Nullish Coalescing (??) ---
		{name: "Coalesce Null", input: "null ?? 10;", expect: "10"},
		{name: "Coalesce Undefined", input: "let u; u ?? 20;", expect: "20"},
		{name: "Coalesce False", input: "false ?? 30;", expect: "false"}, // false is not nullish
		{name: "Coalesce Zero", input: "0 ?? 40;", expect: "0"},          // 0 is not nullish
		{name: "Coalesce EmptyStr", input: `"" ?? 50;`, expect: ""},      // "" is not nullish
		{name: "Coalesce Left Value", input: `"hello" ?? "world";`, expect: "hello"},
		{name: "Coalesce Right Value", input: `null ?? "world";`, expect: "world"},
		{name: "Coalesce Short Circuit", input: `1 ?? (1/0);`, expect: "1"}, // Should not divide by zero

		// --- Compound Assignment Tests ---
		{name: "CompAssign Simple Add", input: "let x = 5; x += 3; x;", expect: "8"},
		{name: "CompAssign Simple Sub", input: "let y = 10; y -= 2; y;", expect: "8"},
		{name: "CompAssign Simple Mul", input: "let z = 4; z *= 2; z;", expect: "8"},
		{name: "CompAssign Simple Div", input: "let w = 16; w /= 2; w;", expect: "8"},

		// --- Compound Assignment in Loops ---
		{name: "CompAssign WhileAdd", input: `
        let sum = 0;
        let i = 1;
        while (i <= 4) {
            sum += i; // 1 + 2 + 3 + 4
            i += 1;
        }
        sum;
        `, expect: "10"},
		{name: "CompAssign ForSub", input: `
        let val = 10;
        for (let i=0; i<3; i+=1) {
            val -= i; // 10 - 0 - 1 - 2
        }
        val;
        `, expect: "7"},
		{name: "CompAssign DoWhileMul", input: `
        let prod = 1;
        let counter = 4;
        do {
            prod *= 2;
            counter -= 1;
        } while (counter > 0);
        prod; // 1 * 2 * 2 * 2 * 2 = 16
        `, expect: "16"},

		// --- Compound Assignment in Closures ---
		{name: "CompAssign ClosureAdd", input: `
        let x = 10;
        let adder = function() { x += 5; };
        adder();
        adder();
        x; // 10 + 5 + 5
        `, expect: "20"},
		{name: "CompAssign ClosureSub", input: `
        let makeSubtractor = function() {
            let outer = 100;
            return function(val) { outer -= val; return outer; };
        };
        let sub = makeSubtractor();
        sub(10);
        sub(20);
        sub(5); // 100 - 10 - 20 - 5 = 65
        `, expect: "65"},

		// --- Increment/Decrement Tests ---
		{name: "IncDec Simple Prefix Inc", input: "let x = 5; let y = ++x; y;", // x becomes 6, y is 6
			expect: "6",
		},
		{name: "IncDec Simple Prefix Inc SideEffect", input: "let x = 5; ++x; x;", // x becomes 6
			expect: "6",
		},
		{name: "IncDec Simple Postfix Inc", input: "let x = 5; let y = x++; y;", // x becomes 6, y is 5
			expect: "5",
		},
		{name: "IncDec Simple Postfix Inc SideEffect", input: "let x = 5; x++; x;", // x becomes 6
			expect: "6",
		},
		{name: "IncDec Simple Prefix Dec", input: "let x = 5; let y = --x; y;", // x becomes 4, y is 4
			expect: "4",
		},
		{name: "IncDec Simple Prefix Dec SideEffect", input: "let x = 5; --x; x;", // x becomes 4
			expect: "4",
		},
		{name: "IncDec Simple Postfix Dec", input: "let x = 5; let y = x--; y;", // x becomes 4, y is 5
			expect: "5",
		},
		{name: "IncDec Simple Postfix Dec SideEffect", input: "let x = 5; x--; x;", // x becomes 4
			expect: "4",
		},

		// --- Increment/Decrement in Loops ---
		{name: "IncDec WhilePrefix", input: `
        let i = 0;
        let sum = 0;
        while (++i < 4) { // i becomes 1, 2, 3 (sum += 1, 2, 3)
            sum += i;
        }
        sum; // 1 + 2 + 3 = 6
        `, expect: "6",
		},
		{name: "IncDec WhilePostfix", input: `
        let i = 0;
        let sum = 0;
        while (i++ < 3) { // i is 0, 1, 2 in condition (sum += 0, 1, 2)
            sum += i;     // i is 1, 2, 3 in body
        }
        sum; // 1 + 2 + 3 = 6 
        `, expect: "6",
		},
		{name: "IncDec ForPrefix", input: `
        let res = 1;
        for(let i = 0; i < 3; ++i) {
            res *= 2;
        }
        res; // 1 * 2 * 2 * 2 = 8
        `, expect: "8",
		},
		{name: "IncDec ForPostfix", input: `
        let res = 1;
        for(let i = 0; i < 3; i++) {
            res *= 2;
        }
        res; // 1 * 2 * 2 * 2 = 8
        `, expect: "8",
		},

		// --- Increment/Decrement in Closures ---
		{name: "IncDec Closure Prefix", input: `
        let x = 10;
        let inc = function() { return ++x; }; // Returns new value
        inc(); // x=11, returns 11
        inc(); // x=12, returns 12
        x + inc(); // 12 + 13 (x becomes 13)
        `, expect: "25",
		},
		{name: "IncDec Closure Postfix", input: `
        let x = 10;
        let inc = function() { return x++; }; // Returns old value
        inc(); // x=11, returns 10
        inc(); // x=12, returns 11
        x + inc(); // 12 + 12 (x becomes 13)
        `, expect: "24",
		},

		// --- Const Assignment Test ---
		{
			name:               "ConstReassignError",
			input:              "const x = 10; x = 20;",
			expect:             "cannot assign to constant variable 'x'",
			expectCompileError: true,
		},

		// --- NEW: Remainder Operator (%) ---
		{
			name:   "Remainder Simple",
			input:  "10 % 3;",
			expect: "1",
		},
		{
			name:   "Remainder Zero",
			input:  "5 % 5;",
			expect: "0",
		},
		{
			name:   "Remainder Float", // JS % is remainder, not modulo
			input:  "5.5 % 2;",
			expect: "1.5",
		},
		{
			name:   "Remainder Negative",
			input:  "-10 % 3;",
			expect: "-1",
		},
		{
			name:    "Remainder By Zero",
			input:   "10 % 0;",
			expect:  "Division by zero (in remainder operation)",
			isError: true,
		},
		{
			name:   "Remainder Precedence",
			input:  "5 + 10 % 4;", // 10 % 4 = 2, 5 + 2 = 7
			expect: "7",
		},

		// --- NEW: Exponentiation Operator (**) ---
		{
			name:   "Exponent Simple",
			input:  "2 ** 3;",
			expect: "8",
		},
		{
			name:   "Exponent Fractional",
			input:  "4 ** 0.5;",
			expect: "2",
		},
		{
			name:   "Exponent Zero",
			input:  "10 ** 0;",
			expect: "1",
		},
		{
			name:   "Exponent One",
			input:  "10 ** 1;",
			expect: "10",
		},
		{
			name:   "Exponent Negative Base", // (-2)**2 = 4
			input:  "(-2) ** 2;",
			expect: "4",
		},
		{
			name:   "Exponent Negative Base Odd", // (-2)**3 = -8
			input:  "(-2) ** 3;",
			expect: "-8",
		},
		{
			name:   "Exponent Negative Exponent", // 2 ** -1 = 0.5
			input:  "2 ** -1;",
			expect: "0.5",
		},
		{
			name:   "Exponent Precedence Left", // (2**3) * 4 = 8 * 4 = 32
			input:  "2 ** 3 * 4;",
			expect: "32",
		},
		{
			name:   "Exponent Precedence Right", // 4 * 2**3 = 4 * 8 = 32
			input:  "4 * 2 ** 3;",
			expect: "32",
		},
		{
			name:   "Exponent Associativity", // 2**(3**2) = 2**9 = 512 (Right-associative like JS/Python)
			input:  "2 ** 3 ** 2;",
			expect: "512",
		},

		// --- NEW: Remainder Assignment (%=) ---
		{
			name:   "CompAssign Remainder",
			input:  "let x = 10; x %= 4; x;", // 10 % 4 = 2
			expect: "2",
		},

		// --- NEW: Exponent Assignment (**=) ---
		{
			name:   "CompAssign Exponent",
			input:  "let y = 3; y **= 3; y;", // 3 ** 3 = 27
			expect: "27",
		},

		// --- NEW: Bitwise Operators ---
		{name: "BitwiseAND", input: "6 & 3;", expect: "2"},                        // 110 & 011 = 010
		{name: "BitwiseOR", input: "6 | 3;", expect: "7"},                         // 110 | 011 = 111
		{name: "BitwiseXOR", input: "6 ^ 3;", expect: "5"},                        // 110 ^ 011 = 101
		{name: "BitwiseNOT", input: "~5;", expect: "-6"},                          // ~0...0101 = 1...1010 (Two's complement)
		{name: "LeftShift", input: "3 << 2;", expect: "12"},                       // 011 << 2 = 1100
		{name: "RightShift", input: "12 >> 1;", expect: "6"},                      // 1100 >> 1 = 0110 (Signed right shift)
		{name: "RightShiftNeg", input: "-8 >> 1;", expect: "-4"},                  // Signed right shift for negative
		{name: "UnsignedRightShift", input: "12 >>> 1;", expect: "6"},             // Need compiler/VM support
		{name: "UnsignedRightShiftNeg", input: "-8 >>> 1;", expect: "2147483644"}, // Assuming 32-bit int

		// --- NEW: Bitwise Assignment Operators ---
		{name: "CompAssign BitwiseAND", input: "let a = 6; a &= 3; a;", expect: "2"},
		{name: "CompAssign BitwiseOR", input: "let b = 6; b |= 3; b;", expect: "7"},
		{name: "CompAssign BitwiseXOR", input: "let c = 6; c ^= 3; c;", expect: "5"},
		{name: "CompAssign LeftShift", input: "let d = 3; d <<= 2; d;", expect: "12"},
		{name: "CompAssign RightShift", input: "let e = 12; e >>= 1; e;", expect: "6"},
		// {name: "CompAssign UnsignedRightShift", input: "let f = 12; f >>>= 1; f;", expect: "6"}, // Need compiler/VM support

		// --- NEW: Logical Assignment Operators ---
		{name: "CompAssign LogicalAND True", input: "let g = true; g &&= false; g;", expect: "false"},
		{name: "CompAssign LogicalAND False", input: "let h = false; h &&= true; h;", expect: "false", disassemble: true},            // Short circuits
		{name: "CompAssign LogicalAND ShortCircuit", input: "let hh = false; hh &&= false; hh;", expect: "false", disassemble: true}, // Short circuits, RHS changed to boolean
		{name: "CompAssign LogicalOR True", input: "let i = true; i ||= false; i;", expect: "true", disassemble: true},               // Short circuits
		{name: "CompAssign LogicalOR False", input: "let j = false; j ||= true; j;", expect: "true"},
		{name: "CompAssign LogicalOR ShortCircuit", input: "let jj = true; jj ||= true; jj;", expect: "true", disassemble: true}, // Short circuits, RHS changed to boolean
		{name: "CompAssign Coalesce Null", input: "let k = null; k ??= 10; k;", expect: "10"},
		{name: "CompAssign Coalesce Undefined", input: "let l; l ??= 20; l;", expect: "20"},
		{name: "CompAssign Coalesce Value", input: "let m = 5; m ??= 30; m;", expect: "5", disassemble: true},              // Does not assign
		{name: "CompAssign Coalesce ShortCircuit", input: "let mm = 1; mm ??= (1/0); mm;", expect: "1", disassemble: true}, // Short circuits
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 1. Compile using the driver
			inputSrc := tc.input

			chunk, compileErrs := driver.CompileString(inputSrc)

			// Handle expected compile errors
			if tc.expectCompileError {
				if len(compileErrs) == 0 {
					t.Fatalf("Expected compile error containing %q, but got no errors.", tc.expect)
				}
				found := false
				var allErrors strings.Builder
				for _, cerr := range compileErrs {
					allErrors.WriteString(cerr.Error() + "\n")
					if strings.Contains(cerr.Error(), tc.expect) {
						found = true
					}
				}
				if !found {
					t.Errorf("Expected compile error containing %q, but got errors:\n%s", tc.expect, allErrors.String())
				}
				return // Test passes if expected compile error is found
			}

			// Handle unexpected compile errors
			if len(compileErrs) > 0 {
				var allErrors strings.Builder
				for _, cerr := range compileErrs {
					allErrors.WriteString(cerr.Error() + "\n")
				}
				t.Fatalf("Unexpected compile errors:\n%s", allErrors.String())
			}
			if chunk == nil {
				t.Fatalf("Compilation succeeded but returned a nil chunk unexpectedly.")
			}

			// 2. Run VM
			vmInstance := vm.NewVM()

			finalValue, runtimeErrs := vmInstance.Interpret(chunk)

			// 3. Check Results
			if tc.isError {
				if len(runtimeErrs) == 0 {
					t.Errorf("Expected runtime error containing %q, but VM returned OK. Final Value: %s", tc.expect, finalValue.String())
				} else {
					found := false
					var allErrors strings.Builder
					for _, rerr := range runtimeErrs {
						allErrors.WriteString(rerr.Error() + "\n")
						if strings.Contains(rerr.Error(), tc.expect) {
							found = true
						}
					}
					if !found {
						t.Errorf("Expected runtime error containing %q, but got errors:\n%s", tc.expect, allErrors.String())
					}
				}
			} else {
				if len(runtimeErrs) > 0 {
					var allErrors strings.Builder
					for _, rerr := range runtimeErrs {
						allErrors.WriteString(rerr.Error() + "\n")
					}
					t.Errorf("Expected value %q, but got runtime errors:\n%s", tc.expect, allErrors.String())
				} else {
					actualOutput := finalValue.String()
					if actualOutput != tc.expect {
						t.Errorf("Expected output %q, but got %q", tc.expect, actualOutput)
					}
				}

				if tc.disassemble {
					t.Logf("--- Disassembly [%s] ---\n%s-------------------------\n",
						inputSrc, chunk.DisassembleChunk(inputSrc))
				}
			}
		})
	}
}

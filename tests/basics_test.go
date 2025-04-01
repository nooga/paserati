package tests

import (
	"bytes"
	"os"
	"paserati/pkg/driver"
	"paserati/pkg/vm"
	"strings"
	"testing"
)

// --- Operator & Literal Matrix Test ---

type matrixTestCase struct {
	name    string
	input   string
	expect  string // Expected output OR expected runtime error substring
	isError bool   // True if expect is a runtime error substring
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
		{name: "InfixAddMismatch", input: `1 + "a";`, expect: "Operands must be two numbers or two strings for '+'", isError: true},
		{name: "InfixSubMismatch", input: `"a" - 1;`, expect: "Operands must be numbers for Subtract", isError: true},

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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 1. Compile using the driver
			// Add implicit return for expression statements if not present
			inputSrc := tc.input
			if !strings.Contains(inputSrc, "return") && !strings.HasSuffix(strings.TrimSpace(inputSrc), "}") {
				// Very basic check: if it doesn't have return and doesn't end with }, assume it's an expression
				// that should be returned. Wrap it.
				// inputSrc = fmt.Sprintf("return (%s);", strings.TrimRight(inputSrc, ";"))
				// Simpler: The VM prints the result of the *last expression statement* anyway.
				// So, no need to add return for simple expression tests.
			}

			chunk, compileErrs := driver.CompileString(inputSrc)

			// We don't expect compile errors for these simple cases
			if len(compileErrs) > 0 {
				t.Fatalf("Unexpected compile errors: %v", compileErrs)
			}
			if chunk == nil {
				t.Fatalf("Compilation succeeded but returned a nil chunk unexpectedly.")
			}

			// 2. Run VM
			vmInstance := vm.NewVM()
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w
			oldStderr := os.Stderr
			rErr, wErr, _ := os.Pipe()
			os.Stderr = wErr

			interpretResult := vmInstance.Interpret(chunk)

			w.Close()
			os.Stdout = oldStdout
			wErr.Close()
			os.Stderr = oldStderr

			var vmStdout bytes.Buffer
			_, _ = vmStdout.ReadFrom(r)
			actualOutput := strings.TrimSpace(vmStdout.String())

			var vmStderr bytes.Buffer
			_, _ = vmStderr.ReadFrom(rErr)
			actualRuntimeError := strings.TrimSpace(vmStderr.String())

			// 3. Check Results
			if tc.isError {
				if interpretResult == vm.InterpretOK {
					t.Errorf("Expected runtime error containing %q, but VM returned InterpretOK. Stdout: %q", tc.expect, actualOutput)
				} else {
					if !strings.Contains(actualRuntimeError, tc.expect) {
						t.Errorf("Expected runtime error containing %q, but got stderr: %q", tc.expect, actualRuntimeError)
					}
				}
			} else {
				if interpretResult != vm.InterpretOK {
					t.Errorf("Expected VM to return InterpretOK, but got %v. Stderr: %q", interpretResult, actualRuntimeError)
				}
				if actualOutput != tc.expect {
					t.Errorf("Expected output %q, but got %q", tc.expect, actualOutput)
				}
			}
		})
	}
}

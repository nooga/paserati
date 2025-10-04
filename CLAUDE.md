# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Paserati is a production-quality TypeScript/JavaScript runtime written in Go that compiles TypeScript directly to bytecode for a register-based virtual machine, bypassing JavaScript transpilation entirely.

**Core Goals**:

1. **Correctness**: Full ECMAScript 262 compliance and TypeScript compatibility
2. **Performance**: Optimized execution with inline caching, shape-based objects, and register allocation

The runtime executes TypeScript directly without lowering to JavaScript, using compile-time type information for optimizations while maintaining full JavaScript semantics for runtime behavior.

## Development Commands

### Build and Run

```bash
# Build the main binary
go build -o paserati cmd/paserati/main.go

# Run the REPL
./paserati

# Execute a TypeScript file
./paserati path/to/script.ts

# Run expression directly
./paserati -e "let x = 42; x * 2"

# Show bytecode output
./paserati -bytecode script.ts

# Show inline cache statistics
./paserati -cache-stats script.ts
```

### Testing

```bash
# Run all tests
go test ./tests/...

# Run script-based tests (most comprehensive) - THIS IS OUR SMOKE TEST
# This test suite MUST always be green - fix issues or mark as FIXME
go test ./tests -run TestScripts

# Run a specific script test
go test ./tests -run "TestScripts/filename.ts"

# Run with verbose output to see individual test names
go test ./tests -v
```

**Important**: The `TestScripts` suite in `tests/scripts_test.go` is our smoke test and should always pass. When something doesn't work, either fix it immediately or mark it as FIXME - never sweep issues under the rug.

### Test262 Compliance Testing

Paserati uses the official ECMAScript Test262 suite to verify language compliance. The `paserati-test262` binary runs these tests with our runtime.

```bash
# Build the test262 runner
go build -o paserati-test262 ./cmd/paserati-test262

# Run all Test262 tests (takes a while)
./paserati-test262 -path ./test262

# Run specific test suite with filtering
./paserati-test262 -path ./test262 -subpath "language/expressions" -filter -timeout 0.5s

# Show suite breakdown with pass rates
./paserati-test262 -path ./test262 -subpath "language/expressions" -suite -filter -timeout 0.5s

# Run a specific test for debugging
./paserati-test262 -path ./test262 -subpath "language/expressions/addition" -pattern "S11.6.1_A3.1_T1.1.js"
```

**Key Flags**:

- `-suite`: Shows pass rates broken down by test suite and subsuite
- `-filter`: Filters out legacy JavaScript patterns (e.g., `with` statements, old ES5 patterns) not relevant for modern TS runtime
- `-timeout`: Set timeout per test (default 5s, use shorter like 0.5s for faster iteration)
- `-subpath`: Limit tests to a specific subdirectory (e.g., "language/expressions", "built-ins/Array")
- `-pattern`: File pattern for test files (default "\*.js")

**Workflow for Fixing Test262 Failures**:

1. **Identify target area**: Run with `-suite -filter` to see pass rates by category

   ```bash
   ./paserati-test262 -path ./test262 -subpath "language/expressions" -suite -filter -timeout 0.5s
   ```

2. **Analyze failure patterns**: Look for common error messages

   ```bash
   ./paserati-test262 -path ./test262 -subpath "language/expressions/addition" -filter | grep "^FAIL" | head -20
   ```

3. **Debug specific failures**: Run individual tests with verbose output

   ```bash
   # Test a specific file
   go run ./cmd/paserati path/to/failing-test.js

   # Or with the test runner
   ./paserati-test262 -path ./test262 -subpath "path/to" -pattern "specific-test.js"
   ```

4. **Create smoke tests**: Add simplified versions to `tests/scripts/` for regression testing

   ```typescript
   // tests/scripts/feature_test.ts
   // expect: value
   console.log("test output");
   ```

5. **Fix the issue**: Make changes following the architecture (lexer → parser → checker → compiler → VM)

6. **Verify improvement**: Re-run the suite to confirm increased pass rate
   ```bash
   ./paserati-test262 -path ./test262 -subpath "language/expressions" -suite -filter
   ```

**Common Test262 Fix Patterns**:

- **Type coercion issues**: JavaScript allows loose type conversions - ensure the type checker permits them and the VM implements correct semantics (e.g., `true + 1` should be `2`)
- **Built-in prototype issues**: Objects must properly inherit from their prototype (e.g., `[] instanceof Array` must be `true`)
- **Property access**: Any value can be used as an object property key (converts to string)
- **Error messages**: Ensure runtime errors match expected ECMAScript behavior
- **ToPrimitive conversions**: Objects in operators should call `valueOf()` or `toString()` methods

**Priority for Fixing**:

1. Focus on categories with lower pass rates but high test counts
2. Look for patterns - fixing one issue often fixes many tests
3. Prioritize correctness over edge cases
4. Keep the smoke test (`TestScripts`) always green

### Practical Test262 Workflow

**Step-by-Step Example Session**:

```bash
# 1. Build the test262 runner
go build -o paserati-test262 ./cmd/paserati-test262

# 2. Get overview of all test categories with pass rates
./paserati-test262 -path ./test262 -subpath "language" -suite -filter -timeout 0.5s

# Example output interpretation:
# language/expressions    11093    3159     6056     1873       16    28.5%
#          ^^^category     ^^^total ^^^pass  ^^^fail  ^^^skip   ^^^timeout ^^^pass%
# This shows expressions has 28.5% pass rate - good target for improvement

# 3. Drill down into a specific category to see subcategories
./paserati-test262 -path ./test262 -subpath "language/expressions" -suite -filter -timeout 0.5s | grep "language/expressions"

# Example output:
# language/expressions/addition         48      29       18        1        0    60.4%
# language/expressions/instanceof       43      19       23        1        0    44.2%
# language/expressions/object         1170     407      703       60        0    34.8%
# Pick the one with high test count and lower pass rate

# 4. Look at actual failures in that category
./paserati-test262 -path ./test262 -subpath "language/expressions/object" -filter -timeout 0.5s | grep "^FAIL" | head -20

# Example output analysis:
# FAIL 14/1170 test262/.../S11.1.5_A1.1.js - test failed: ... object instanceof Object === true
# FAIL 15/1170 test262/.../S11.1.5_A1.2.js - test failed: ... object instanceof Object === true
# Pattern detected: instanceof Object is failing - this is systemic!

# 5. Test the issue manually to understand it
go run ./cmd/paserati -e 'const obj = {}; console.log(obj instanceof Object);'
# Output: false
# Expected: true
# Now we know what to fix!

# 6. Look at a specific failing test to understand requirements
cat test262/test/language/expressions/object/S11.1.5_A1.1.js | head -30

# 7. After implementing a fix, verify it works manually
go run ./cmd/paserati -e 'const obj = {}; console.log(obj instanceof Object);'
# Output: true
# Good! Now test it didn't break smoke tests

# 8. Run smoke tests to ensure no regressions
go test ./tests -run TestScripts
# All tests should pass

# 9. Measure improvement in Test262
./paserati-test262 -path ./test262 -subpath "language/expressions/object" -filter -timeout 0.5s | tail -10
# Before: Passed: 407 (34.8%)
# After:  Passed: 418 (35.7%)  <- 11 more tests passing!

# 10. Check overall improvement
./paserati-test262 -path ./test262 -subpath "language/expressions" -suite -filter -timeout 0.5s | tail -10
# Track the overall pass rate increase
```

**Interpreting Test Results**:

When you see a failure like:

```
FAIL 14/1170 test262/test/language/expressions/object/S11.1.5_A1.1.js - test failed: Runtime Error at 1:1: Uncaught exception: Test262Error: #2: var object = {}; object instanceof Object === true
```

Break it down:

- `FAIL 14/1170`: Test #14 out of 1,170 total tests
- `test262/.../S11.1.5_A1.1.js`: The specific test file
- `Runtime Error at 1:1`: Error occurred at line 1, column 1
- `Uncaught exception: Test262Error`: The test threw an error (test assertion failed)
- `#2: var object = {}; object instanceof Object === true`: The assertion message tells you what should be true

**Common Patterns to Look For**:

1. **Multiple similar failures** = Systemic issue worth fixing

   ```bash
   # Count failures by error message
   ./paserati-test262 -path ./test262 -subpath "language/expressions" -filter 2>&1 | \
     grep "test failed:" | sed 's/.*test failed: //' | sort | uniq -c | sort -rn | head -10
   ```

2. **High failure count in one subcategory** = Missing feature or broken implementation

   ```bash
   # See which subcategories have most failures
   ./paserati-test262 -path ./test262 -subpath "language/expressions" -suite -filter -timeout 0.5s | \
     grep "language/expressions/" | sort -k3 -rn | head -10
   ```

3. **Timeout issues** = Infinite loop or performance problem
   ```bash
   # Find tests that timeout
   ./paserati-test262 -path ./test262 -subpath "language/expressions" -filter -timeout 0.5s 2>&1 | \
     grep "TIMEOUT"
   ```

**Quick Commands Reference**:

```bash
# Get overall pass rate for expressions
./paserati-test262 -path ./test262 -subpath "language/expressions" -suite -filter -timeout 0.5s | grep "GRAND TOTAL"

# Test a single file for debugging
./paserati-test262 -path ./test262 -subpath "language/expressions/addition" -pattern "S11.6.1_A3.1_T1.1.js" -timeout 0.5s

# Find tests by error keyword
./paserati-test262 -path ./test262 -subpath "language/expressions" -filter 2>&1 | grep "instanceof"

# Compare before/after (save results to files)
./paserati-test262 -path ./test262 -subpath "language/expressions" -suite -filter > before.txt
# ... make fixes ...
./paserati-test262 -path ./test262 -subpath "language/expressions" -suite -filter > after.txt
diff before.txt after.txt
```

### Adding New Language Features

When implementing new operators, statements, or language constructs, follow this pipeline:

1. **Lexer** (`pkg/lexer/lexer.go`): Add token types and keyword mappings
2. **Parser** (`pkg/parser/`): Add AST nodes (`ast.go`) and parsing logic (`parser.go`)
   - For complex features, create dedicated `parse_*.go` files (e.g., `parse_class.go`)
3. **Type Checker** (`pkg/checker/`): Add type checking logic in appropriate files
   - Create dedicated checker files for complex features (e.g., `class.go`, `narrowing.go`)
4. **Compiler** (`pkg/compiler/`): Add bytecode generation in `compile_*.go` files
   - Create dedicated compilation files for major features (e.g., `compile_class.go`)
5. **VM** (`pkg/vm/`): Add bytecode execution logic and any new opcodes
6. **Tests**: Create test scripts in `tests/scripts/` with expectation comments

## Architecture Overview

### Code Structure

The codebase is organized into distinct packages:

- `pkg/lexer/` - Lexical analysis and tokenization
- `pkg/parser/` - AST construction and syntax analysis
- `pkg/checker/` - Type checking and semantic analysis
- `pkg/compiler/` - Bytecode generation and optimization
- `pkg/vm/` - Virtual machine and runtime execution
- `pkg/types/` - Type system definitions and operations
- `pkg/builtins/` - Built-in objects and functions
- `pkg/driver/` - Main compilation pipeline orchestration
- `pkg/errors/` - Error types and handling
- `pkg/source/` - Source code management

**Important**: When implementing major features, create new files in the appropriate package rather than editing long existing files. For example:

- Parser features: Create `parse_feature.go` in `pkg/parser/`
- Type checking: Create `feature.go` in `pkg/checker/`
- Compilation: Create `compile_feature.go` in `pkg/compiler/`

### Compilation Pipeline

The codebase follows a traditional compiler pipeline:

- **Lexer**: Tokenizes TypeScript source code
- **Parser**: Builds AST using Pratt parser for expressions
- **Type Checker**: Performs TypeScript-compliant type checking with type narrowing
- **Compiler**: Generates bytecode for register-based VM with register allocation
- **VM**: Executes bytecode with inline caching and optimized object representations

### Core Components

**Driver Package** (`pkg/driver/`): Entry point that orchestrates the compilation pipeline. The `Paserati` struct maintains persistent state between evaluations for REPL functionality.

**Virtual Machine** (`pkg/vm/`): Register-based VM with two object representations:

- `PlainObject`: Shape-based optimization similar to V8's hidden classes
- `DictObject`: Simple hash-map based storage for dynamic objects
- `ArrayObject`: Specialized array representation with `length` property

**Type System** (`pkg/types/`): Comprehensive TypeScript type system supporting:

- Primitive types, literal types, union/intersection types
- Object types, interfaces with inheritance
- Function types with overloads, optional/default parameters
- Tuple types with contextual typing integration
- **Generic types** with constraints and type inference
- **Class types** represented as constructor function types
- Type narrowing and control flow analysis

**Built-ins** (`pkg/builtins/`): Modern architecture with prototype registry system. Each built-in type (Array, String, Math, etc.) has consolidated implementation with both runtime methods and type information.

### Testing Framework

**Script-Based Testing**: The primary testing mechanism uses TypeScript files in `tests/scripts/` with special comment annotations:

```typescript
// expect: value                    // Expected output value
// expect_runtime_error: message    // Expected runtime error
// expect_compile_error: message    // Expected compile error
```

Test files are automatically discovered and executed by `tests/scripts_test.go`.

## Key Implementation Patterns

### Adding New Operators

1. Add token type to lexer with appropriate precedence
2. Register infix/prefix parser function
3. Add type checking case in checker's operator switch
4. Add compilation case in compiler's operator switch
5. Add VM execution case with proper bytecode
6. Create test files covering success and error cases

### Object Property Operations

The VM supports three object types with different property access patterns:

- Use `HasOwn()` method for property existence
- Use `GetOwn()`/`SetOwn()` for property access
- Arrays handle numeric indices specially

### Type Checking Integration

The type checker uses widened types for compatibility checking and maintains separate environments for type narrowing in control flow branches.

### Bytecode Generation

The compiler uses register allocation with automatic cleanup via defer patterns. Each compilation function manages its temporary registers and frees them on completion.

**Key VM Implementation Details**:

- **Type Coercion**: The VM implements JavaScript's type coercion via `ToFloat()`, `ToString()`, `ToBoolean()` methods on `Value`
- **ToPrimitive**: For objects in operators, the VM calls `valueOf()` or `toString()` methods via the `vm.toPrimitive()` function
- **instanceof**: Walks the prototype chain comparing to constructor's `.prototype` property
- **Property Access**: Any value can be used as an object key (gets converted to string)
- **Error Messages**: Use `ValueType.String()` method to get human-readable type names in error messages

## Feature Status

See `docs/bucketlist.md` for comprehensive implementation status.

**Recently Completed (Latest ECMAScript Compliance Work)**:

- ✅ Type coercion for arithmetic operators (boolean, null, undefined to number)
- ✅ ToPrimitive implementation with valueOf()/toString() calling
- ✅ instanceof operator for all built-in types (Object, Array, RegExp, etc.)
- ✅ Property access with any value as key (converts to string)
- ✅ Proper error messages with human-readable type names
- ✅ Super expressions with spread arguments
- ✅ Object addition via valueOf() methods

**Core Language Features**:

- **Classes** - Declarations, constructors, methods, inheritance, super
- **Generics** - Complete generic type system with constraints and inference
- **Regular expressions** - Full RegExp support with literal syntax and methods
- Type assertions (`as` operator)
- Property existence checking (`in` operator)
- Spread syntax in function calls and array/object literals
- Comprehensive type narrowing and control flow analysis
- Interface inheritance
- Optional/default parameters
- Rest parameters

**Current Test262 Compliance** (as of recent fixes):

- Expressions: ~28.5% pass rate (3,159/11,093 tests)
- Object expressions: ~34.8% pass rate
- instanceof tests: ~44.2% pass rate
- Addition tests: ~60.4% pass rate

## Development Notes

### Build and Debug

- **Always rebuild the binary before testing**: `go build -o paserati cmd/paserati/main.go`
- Use `go build ./...` to verify all packages compile
- Main components have debug flags in their `.go` files:
  - `pkg/lexer/lexer.go`: `const debugLexer = false`
  - `pkg/parser/parser.go`: `const debugParser = false`
  - `pkg/checker/checker.go`: `const debugChecker = false`
  - `pkg/compiler/compiler.go`: `const debugCompiler = false`
  - Enable these flags (set to `true`) to see detailed debug information when troubleshooting

### Quality Standards

**Core Principles**:

- **Goal**: Production-quality runtime with full ECMAScript 262 compliance
- **Correctness First**: Get the semantics right before optimizing
- **Performance Matters**: Use profiling to guide optimizations, not premature optimization
- **No Shortcuts**: Either implement features correctly or don't implement them

**Language Compliance**:

- This is a **TypeScript-first runtime** that executes TS directly without transpiling to JS
- We target **latest ECMAScript** specification for JavaScript compatibility
- Type checker can be disabled (for Test262) to ensure pure JavaScript compliance
- When type checking is enabled, we use type information for optimizations (number representation, monomorphization, etc.)

**Code Quality**:

- Error messages should match TypeScript compiler output for familiarity (for type errors) or ECMAScript spec (for runtime errors)
- **When something doesn't work**: Either fix it immediately or mark it as FIXME - never sweep issues under the rug
- **Test quality**: Our smoke test (`TestScripts`) must always be green
- Use Test262 to verify correctness against the ECMAScript specification
- Performance optimizations: inline caching, shape-based objects, register allocation, specialized bytecode operations

**JavaScript Semantics**:

- Implement proper type coercion (ToPrimitive, ToNumber, ToString, ToBoolean)
- Respect prototype chains for all built-in types
- Handle edge cases correctly (e.g., `undefined` as object key becomes `"undefined"`)
- Follow ECMAScript abstract operations exactly

### Implementation Guidelines

- **Important**: When implementing classes, properties must be explicitly declared in the class body (TypeScript requirement)
- **Important**: Never use git commands with the -i flag (like git rebase -i or git add -i) since they require interactive input which is not supported
- NEVER create files unless they're absolutely necessary for achieving your goal
- ALWAYS prefer editing an existing file to creating a new one, unless implementing a major feature
- NEVER proactively create documentation files (\*.md) or README files. Only create documentation files if explicitly requested by the User

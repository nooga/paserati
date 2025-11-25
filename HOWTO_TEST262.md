# How to Successfully Increase Test262 Coverage

This guide documents the practical workflow for fixing Test262 failures in Paserati.

## The Core Loop

The goal is to iteratively reduce the "delta" between the baseline (last commit) and your current working copy.

1.  **Establish Baseline**: The `baseline.txt` file is regenerated on every commit. It represents the "known state" of the project.
2.  **Run Tests & Check Delta**:

    ```bash
    # Rebuild runner first!
    go build -o paserati-test262 ./cmd/paserati-test262/

    # Run tests and compare with baseline
    ./paserati-test262 -diff baseline.txt
    ```

    - **Green (+)**: You fixed a test!
    - **Red (-)**: You broke a test (regression).
    - **Goal**: Maximize Green, eliminate Red.

## The Bugfixing Workflow

### 1. Identify a Target Suite

Don't try to fix everything at once. Pick a specific language feature or directory.

- Focus on `test262/test/language/...`
- Use `analyze_failures.sh` to find common error patterns.
- Look at `docs/test262_fix_opportunities.md` for low-hanging fruit.

### 2. Reproduce Locally

Running the full suite is slow. Create a minimal reproduction.

1.  **Find a failing test**:
    ```bash
    ./paserati-test262 -subpath language/expressions/assignment -limit 10 -timeout 0.2s
    ```
2.  **Isolate it**:
    Run just that test to confirm failure:
    ```bash
    ./paserati-test262 -pattern 'test-name.js'
    ```
3.  **Create a Minimal Repro**:
    Create a file in `tests/scripts/local_repro.ts`.
    - **Crucial**: Test262 ignores type checking!
    - When running your repro with the main binary, **ALWAYS use `--no-typecheck`**:
      ```bash
      go build -o paserati cmd/paserati/main.go
      ./paserati --no-typecheck tests/scripts/local_repro.ts
      ```
    - _Note_: Fixes to the type checker usually do NOT help with Test262 compliance. Focus on the Compiler and VM.

### 3. Debugging

When you have a minimal repro, turn on the lights.

- **Enable Debug Flags**: Edit the constants at the top of these files to `true`:
  - `pkg/compiler/compiler.go` (`debugCompiler`)
  - `pkg/vm/vm.go` (`debugVM`)
  - `pkg/parser/parser.go` (`debugParser`)
  - `pkg/lexer/lexer.go` (`debugLexer`)
- **Rebuild and Run**:
  ```bash
  go build -o paserati cmd/paserati/main.go
  ./paserati --no-typecheck tests/scripts/local_repro.ts
  ```
- **Analyze Output**: Look at the bytecode disassembly and execution trace.

### 4. Fix and Verify

1.  Modify the code (Compiler/VM).
2.  Run your local repro: `./paserati --no-typecheck tests/scripts/local_repro.ts`
3.  Run the specific Test262 test: `./paserati-test262 -pattern 'test-name.js'`
4.  **Check for Regressions**:
    ```bash
    ./paserati-test262 ... -timeout 0.2s -diff baseline.txt
    ```
    Ensure you haven't broken other things.

## Tips & Tricks

- **Rebuild Often**: The runner and binary are not auto-rebuilt.
  - `go build -o paserati-test262 ./cmd/paserati-test262/`
  - `go build -o paserati cmd/paserati/main.go`
- **Or use `go run`**:
  - `go run ./cmd/paserati-test262 ...`
- **Disassembly**: The VM prints disassembly when `debugVM` is on, or use `./paserati -bytecode file.ts`.
- **Ignore Types**: Remember, Test262 is about runtime correctness (JavaScript behavior), not TypeScript rules.

## Finding Opportunities with `paserati-analyze`

The `paserati-analyze` tool clusters failures by error pattern, making it easy to find high-impact fixes.

### Basic Usage

```bash
# Build the analyzer
go build -o paserati-analyze ./cmd/paserati-analyze/

# Analyze a specific subsuite
./paserati-test262 -subpath language/expressions/object -json | ./paserati-analyze

# Analyze multiple areas
./paserati-test262 -subpath language/expressions -json | ./paserati-analyze
```

### Output Interpretation

The tool outputs failure groups sorted by count:

```
[42] undefined is not an object
  - test262/test/language/expressions/object/foo.js
  - test262/test/language/expressions/object/bar.js
  - test262/test/language/expressions/object/baz.js
  ... and 39 more
```

The `[42]` means 42 tests fail with this normalized error. These are your **high-yield targets** - fixing the root cause could pass many tests at once.

### Strategy

1. **Pick a subsuite** with moderate pass rate (70-90%) - these often have easy wins
2. **Run the analyzer** on that subsuite
3. **Focus on the largest clusters** - one fix, many test passes
4. **Avoid scattered errors** - if each test fails differently, the suite may need foundational work

### Example Workflow

```bash
# Find opportunities in expressions/object (91.1% pass rate)
./paserati-test262 -timeout 0.2s -subpath language/expressions/object -json | ./paserati-analyze

# Find opportunities in expressions/class (90.1% pass rate)
./paserati-test262 -timeout 0.2s -subpath language/expressions/class -json | ./paserati-analyze

# Find opportunities in expressions/array (86.5% pass rate)
./paserati-test262 -timeout 0.2s -subpath language/expressions/array -json | ./paserati-analyze
```

Suites at 85-92% pass rate often have clusters of related bugs. Suites below 70% may require more fundamental fixes.

## Tooling Wishlist (Current Limitations)

- **Disassembly**: Currently dumps everything or top-level. Hard to inspect just _one_ function or generator.

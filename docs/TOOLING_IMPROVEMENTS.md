# Test262 Tooling Improvement Plan

This document outlines proposed improvements to the Paserati Test262 tooling to enable more autonomous and efficient bugfixing loops.

## 1. Enhanced Failure Analysis (`cmd/paserati-analyze`)

**Current State**: `analyze_failures.sh` uses `grep` and `sed` to count exact error strings. This misses variations (e.g., "Expected 1 to be 2" vs "Expected 3 to be 4") and doesn't group by root cause.

**Proposal**: Create a Go-based analysis tool (`cmd/paserati-analyze`) that parses `paserati-test262` output.

### Features:
*   **Fuzzy Grouping**: Group errors by template.
    *   *Input*: `Runtime Error: Expected SameValue(«1», «2») to be true`
    *   *Group*: `Runtime Error: Expected SameValue(«*», «*») to be true`
*   **Stack Trace Clustering**: Group failures that crash at the same source location (file/line) or Go function.
*   **Regression Attribution**: If running with `-diff`, attribute regressions to specific changes if possible (advanced).
*   **Output Formats**: JSON output for automated agents to consume, and human-readable summaries.

## 2. Targeted Disassembly

**Current State**: Disassembly is all-or-nothing (top-level chunk) or requires enabling verbose `debugVM` flags which flood the console with execution traces.

**Proposal**: Add targeted disassembly flags to the `paserati` binary.

### Features:
*   **`-disasm-filter <regex>`**: Only disassemble functions whose names match the pattern.
    *   *Implementation*: In `CompileProgram`, when generating `Chunk`s for functions, check the name against the filter before printing.
*   **Recursive Disassembly**: `DisassembleChunk` currently only prints the linear bytecode of the chunk. It should optionally recurse into nested functions (OpClosure targets) to show the full tree.
*   **Source Mapping**: Print the original source line next to the bytecode instruction (already partially supported via `Lines` array, but could be improved with source snippets).

## 3. Autonomous Loop Integration

**Goal**: Enable an agent to run the loop without human intervention.

### Workflow:
1.  **Agent Command**: `paserati-test262 -dump current.json -format json`
2.  **Analysis**: Agent reads `current.json`, identifies top failure cluster (e.g., "Array.prototype.map").
3.  **Reproduction**: Agent generates `tests/scripts/repro_gen.ts` based on the failing test content.
4.  **Debugging**: Agent runs `paserati -disasm-filter "map" tests/scripts/repro_gen.ts` to see relevant bytecode.
5.  **Fix & Verify**: Agent applies fix, re-runs repro, then re-runs test262 subset.

## 4. Immediate Action Items

1.  **Update `paserati` binary**: Add `-disasm` flag that works even without `debugVM` constant.
2.  **Update `paserati-test262`**: Ensure it can output structured data (JSON) for easier parsing.

# Zero-Cost Debugger Design: The `OpDebug` Trap

## Objective
Add a debugger to the Paserati VM that allows breakpoints and stepping **without any performance penalty when the debugger is disabled or inactive**.

## The Problem
Typical debuggers add checks in the hot execution loop:
```go
for {
    if vm.DebugMode { ... } // Performance penalty on EVERY instruction!
    switch code[ip] { ... }
}
```
We must avoid this branch in the critical path.

## The Solution: Opcode Replacement (`OpDebug`)

We will use a "Trap" mechanism similar to hardware debuggers (e.g., `int 3` on x86).

### 1. New Opcode: `OpDebug`
We reserve a special opcode (e.g., `255`) called `OpDebug`.

### 2. Setting a Breakpoint
When the user sets a breakpoint at instruction `IP`:
1.  The VM saves the *original* byte at `code[IP]` into a side-table: `vm.breakpoints[IP] = originalOp`.
2.  The VM overwrites `code[IP]` with `OpDebug`.

**Performance Impact**: Zero. The code at other locations is unchanged. The VM loop has no extra checks.

### 3. Hitting a Breakpoint
When the VM encounters `OpDebug`:
1.  It executes the `case OpDebug:` block.
2.  This block simply returns a new status: `InterpretBreakpoint`.
    ```go
    case OpDebug:
        return InterpretBreakpoint, Undefined
    ```
3.  The VM loop exits immediately. The Go stack unwinds (or pauses if we structure it right, but returning is safest/simplest).

### 4. The Debug Driver (External)
The logic for "what to do next" lives *outside* the hot loop, in the driver (e.g., the REPL or DAP server).

**To Resume Execution ("Continue"):**
1.  **Restore**: The driver temporarily writes the `originalOp` back to `code[IP]`.
2.  **Step**: The driver calls `vm.Step()` (a new method that executes exactly one instruction).
3.  **Re-patch**: The driver writes `OpDebug` back to `code[IP]` (to keep the breakpoint active).
4.  **Run**: The driver calls `vm.Run()`.

### 5. Stepping (Step Over/Into)
Since we are already "stopped" (outside `vm.Run()`), we can implement stepping by:
1.  **Single Step**: Call `vm.Step()` once.
2.  **Step Over**:
    *   Analyze current opcode.
    *   Calculate target IP (next instruction or jump target).
    *   Place a *temporary* `OpDebug` at the target IP.
    *   Call `vm.Run()`.
    *   When it returns (hit temp breakpoint), restore original code.

## Implementation Details

### `pkg/vm/bytecode.go`
*   Add `OpDebug OpCode = 255`.

### `pkg/vm/vm.go`
*   Add `InterpretBreakpoint` to `InterpretResult`.
*   Add `breakpoints map[int]byte` to `VM` struct.
*   Add `SetBreakpoint(ip int)` and `ClearBreakpoint(ip int)`.
*   Add `Step()` method (executes one instruction, effectively a stripped-down `run()` that breaks after one loop).
    *   *Optimization*: `Step()` can just call `run()` with a `limit=1` parameter? Or `run()` takes a `stepMode bool`?
    *   *Better*: `run()` takes a `steps int` argument. `-1` for infinite.
    *   *Zero-Cost Check*: `if steps > 0 { steps--; if steps == 0 { return InterpretStep, ... } }`.
    *   *Wait*: This adds a check to the loop.
    *   *Alternative*: `Step()` is a separate function that copies the switch statement. Code duplication, but guarantees zero impact on `Run()`. Given the complexity of `Run()`, maybe we just accept the `steps` check?
    *   *Refined*: The `steps` check is only one integer decrement and branch. Modern CPUs predict this "not taken" very well.
    *   *Even Better*: `Step()` uses the `OpDebug` mechanism! It patches the *next* instruction with `OpDebug`, runs, then unpatches. No changes to `Run()` loop needed!

### The "Step via Trap" Strategy
To step one instruction without modifying `Run()`:
1.  Decode current instruction at `IP`.
2.  Determine `NextIP` (size of current op + operands, or jump target).
3.  Save code at `NextIP`.
4.  Patch `NextIP` with `OpDebug`.
5.  Call `Run()`.
6.  VM executes current op, jumps to `NextIP`, hits `OpDebug`, returns.
7.  Restore code at `NextIP`.

*Note*: This is tricky for branching instructions. We might need to patch *both* possible destinations (fallthrough and jump target).

## Summary
*   **No overhead when disabled.**
*   **Minimal overhead when enabled** (only at actual breakpoints).
*   **Clean architecture**: VM stays dumb, Driver handles logic.

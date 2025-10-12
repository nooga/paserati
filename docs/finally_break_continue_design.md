# Finally Block Break/Continue Implementation Design

## Problem Statement

ECMAScript requires that `finally` blocks execute before control flow statements (break, continue, return) take effect. Currently, Paserati does not correctly handle break/continue statements inside try-finally blocks.

### Expected Behavior

```javascript
var fin = 0;
while (true) {
    try {
        break;
    } finally {
        fin = 1;  // MUST execute before break takes effect
    }
}
// Expected: fin = 1
```

### Current Behavior

The break jumps directly to the loop exit, bypassing the finally block entirely.

## Root Cause Analysis

### Current Compilation Model

The compiler uses a placeholder-patching approach for jumps:

1. Break statement emits: `OpJump <placeholder>`
2. Placeholder position added to `LoopContext.BreakPlaceholderPosList`
3. When loop finishes compiling, all placeholders are patched to the loop exit PC

This works fine without finally blocks, but breaks down when finally is involved.

### Why Previous Attempt Failed

The naive approach was:

```
try:
  break; → jump @finally
finally:
  ...
  OpHandlePending
  jump @loop_exit  // Deferred break placeholder
```

This fails in two critical scenarios:

#### Scenario 1: Loop Inside Try Block

```javascript
try {
    for (let item of items) {
        if (item === "b") break;  // Break targets inner loop
    }
} finally {
    console.log("finally");
}
```

Compilation order:
1. Compile try block
2. Compile for-of loop (lines 2-5)
3. Compile break statement → adds placeholder to loop's list
4. **For-of loop FINISHES** → patches all placeholders (including break)
5. Try block finishes
6. Compile finally block
7. Emit deferred break placeholder after finally
8. **PROBLEM**: Loop already finished compiling at step 4, will never patch this placeholder

Result: Unpatched 0xFFFF placeholder → "Unknown opcode 255" error

#### Scenario 2: Multiple Breaks with Different Targets

```javascript
while (c < 3) {
    try {
        if (x) break;    // Break to outer while
        if (y) continue; // Continue outer while
    } finally {
        fin++;
    }
}
```

Both break and continue jump to finally, but after finally they need to go to DIFFERENT places:
- Break should jump to loop exit
- Continue should jump to loop start

With the naive approach:
```
try:
  if (x) jump @finally  // break
  if (y) jump @finally  // continue
finally:
  ...
  OpHandlePending
  jump @loop_exit    // Only one jump!
  jump @loop_start   // This is unreachable!
```

Both paths execute the same jump after finally, causing incorrect behavior.

#### Scenario 3: Conditional Break

```javascript
while (i < 3) {
    try {
        i++;
        if (i >= 2) break;
        // Normal execution continues
    } finally {
        fin++;
    }
}
```

Problem: Normal exit from try also goes through finally. After finally:
- If break was taken: should exit loop
- If normal exit: should continue after try-finally

With naive approach, both paths execute the same deferred break jump.

## Solution: Completion Record Mechanism

### ECMAScript Specification Approach

ECMAScript defines **Completion Records** with three components:
- `[[Type]]`: normal, break, continue, return, throw
- `[[Value]]`: The value (for return) or empty
- `[[Target]]`: The target label (for break/continue) or empty

When a break/continue/return occurs in a try block with finally:
1. Create a completion record
2. Execute finally block
3. After finally, apply the completion record

### Implementation Strategy for Paserati

We need a lightweight mechanism to track "what to do after finally executes". This requires:

1. **Runtime State**: A completion stack in the VM
2. **Compiler Support**: Emit opcodes to push/pop completions
3. **Handler Opcode**: OpHandlePending checks completion and acts accordingly

### Detailed Design

#### 1. VM Data Structures

Add to `vm.go`:

```go
type CompletionType int

const (
	CompletionNormal CompletionType = iota
	CompletionBreak
	CompletionContinue
	CompletionReturn
	CompletionThrow
)

type Completion struct {
	Type     CompletionType
	Value    Value  // For return
	TargetPC int    // For break/continue (absolute PC)
}

// In VM struct:
type VM struct {
    // ... existing fields ...

    // Completion stack for try-finally control flow
    completionStack []Completion
}
```

#### 2. New Opcodes

Add to `bytecode.go`:

```go
// Opcode 107: Push completion record for break
// Operands: TargetPC(16-bit placeholder)
OpPushBreak OpCode = 107

// Opcode 108: Push completion record for continue
// Operands: TargetPC(16-bit placeholder)
OpPushContinue OpCode = 108

// Note: OpReturnFinally already exists and sets pending return
```

#### 3. Compilation Strategy

##### When Compiling Break/Continue in Try-Finally

```go
func (c *Compiler) compileBreakStatement(node *parser.BreakStatement, hint Register) error {
    targetContext := /* find target loop */

    if len(c.finallyContextStack) > 0 {
        // Inside try-finally: push completion and jump to finally
        finallyCtx := c.finallyContextStack[len(c.finallyContextStack)-1]

        // Emit: OpPushBreak <placeholder>
        pos := len(c.chunk.Code)
        c.emitOpCode(vm.OpPushBreak, node.Token.Line)
        placeholderPos := len(c.chunk.Code)
        c.emitUint16(0xFFFF)  // Placeholder for target PC

        // Add placeholder to loop's list so it gets patched when loop finishes
        targetContext.BreakPlaceholderPosList = append(
            targetContext.BreakPlaceholderPosList,
            placeholderPos,
        )

        // Jump to finally
        jumpPos := c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)
        finallyCtx.JumpToFinallyPlaceholders = append(
            finallyCtx.JumpToFinallyPlaceholders,
            jumpPos,
        )
    } else {
        // No finally: normal break
        placeholderPos := c.emitPlaceholderJump(vm.OpJump, 0, node.Token.Line)
        targetContext.BreakPlaceholderPosList = append(
            targetContext.BreakPlaceholderPosList,
            placeholderPos,
        )
    }
}
```

**Key Insight**: The placeholder in OpPushBreak WILL get patched by the loop because:
- The loop is still being compiled (it contains the try-finally)
- When the loop finishes, it patches ALL positions in BreakPlaceholderPosList
- This includes the placeholder embedded in OpPushBreak

##### Patching OpPushBreak/OpPushContinue Placeholders

The loop's patching code needs to handle these opcodes:

```go
func (c *Compiler) patchBreakPlaceholders(loopCtx *LoopContext, exitPC int) {
    for _, placeholderPos := range loopCtx.BreakPlaceholderPosList {
        op := vm.OpCode(c.chunk.Code[placeholderPos])

        if op == vm.OpPushBreak {
            // Patch the embedded placeholder in OpPushBreak
            // Format: OpPushBreak(1 byte) + TargetPC(2 bytes)
            operandPos := placeholderPos + 1
            c.patchJumpToTarget(operandPos-1, exitPC) // Use existing helper
        } else if op == vm.OpJump {
            // Regular jump placeholder
            c.patchJumpToTarget(placeholderPos, exitPC)
        }
    }
}
```

Wait, that's not right. Let me reconsider...

Actually, the placeholder position we store should be the POSITION OF THE PLACEHOLDER BYTES, not the opcode. Let me revise:

```go
// When emitting OpPushBreak:
c.emitOpCode(vm.OpPushBreak, node.Token.Line)
placeholderPos := len(c.chunk.Code)  // Position of the 2-byte placeholder
c.emitUint16(0xFFFF)

// Store placeholderPos - but we need to know it's OpPushBreak, not OpJump
// So we need a different data structure...
```

**Problem**: The BreakPlaceholderPosList stores int positions, but we need to know the opcode type to patch correctly.

**Solution**: Create a new data structure:

```go
type JumpPlaceholder struct {
    Position int      // Byte position in code
    Type     OpCode   // OpJump, OpPushBreak, OpPushContinue
}

type LoopContext struct {
    // ... existing fields ...
    BreakPlaceholders    []JumpPlaceholder
    ContinuePlaceholders []JumpPlaceholder
}
```

Then patching becomes:

```go
func (c *Compiler) patchBreakPlaceholders(loopCtx *LoopContext, exitPC int) {
    for _, ph := range loopCtx.BreakPlaceholders {
        switch ph.Type {
        case vm.OpJump:
            c.patchJumpToTarget(ph.Position, exitPC)
        case vm.OpPushBreak:
            // Patch the 2-byte operand
            c.patchOperand16(ph.Position+1, exitPC)
        }
    }
}

func (c *Compiler) patchOperand16(pos int, targetPC int) {
    // Calculate relative offset from the byte AFTER the operand
    offsetFrom := pos + 2
    offset := targetPC - offsetFrom

    if offset > math.MaxInt16 || offset < math.MinInt16 {
        panic(fmt.Sprintf("Jump offset %d exceeds 16-bit limit", offset))
    }

    c.chunk.Code[pos] = byte(int16(offset) >> 8)
    c.chunk.Code[pos+1] = byte(int16(offset) & 0xFF)
}
```

Wait, I'm overcomplicating this. Let me think...

Actually, OpPushBreak embeds a jump offset, just like OpJump. So we can use the SAME patching logic! The only difference is where the offset bytes are located relative to the opcode.

For OpJump: `opcode(1) + offset(2)` = offset at position+1
For OpPushBreak: `opcode(1) + offset(2)` = offset at position+1 (same!)

So we can use the existing patchJumpToTarget! But we need to store the OPCODE POSITION, not the offset position.

Let me revise:

```go
// Emit OpPushBreak and store the OPCODE position
opcodePos := len(c.chunk.Code)
c.emitOpCode(vm.OpPushBreak, node.Token.Line)
c.emitUint16(0xFFFF)  // Placeholder

// Store the opcode position
targetContext.BreakPlaceholderPosList = append(
    targetContext.BreakPlaceholderPosList,
    opcodePos,
)
```

And patching:

```go
func (c *Compiler) patchJumpToTarget(placeholderPos int, targetPC int) {
    op := vm.OpCode(c.chunk.Code[placeholderPos])

    var operandStartPos int
    if op == vm.OpJumpIfFalse || op == vm.OpJumpIfUndefined || /* ... */ {
        operandStartPos = placeholderPos + 2 // Skip opcode + register
    } else if op == vm.OpPushBreak || op == vm.OpPushContinue {
        operandStartPos = placeholderPos + 1 // Skip opcode only
    } else { // OpJump
        operandStartPos = placeholderPos + 1 // Skip opcode only
    }

    jumpInstructionEndPos := operandStartPos + 2
    offset := targetPC - jumpInstructionEndPos

    // Write offset...
}
```

This works! OpPushBreak is treated just like OpJump for patching purposes.

#### 4. VM Execution

##### OpPushBreak

```go
case OpPushBreak:
    targetPCHi := code[ip]
    targetPCLo := code[ip+1]
    ip += 2

    // Calculate absolute target PC from relative offset
    offsetFrom := ip  // Position after the operand
    offset := int16(uint16(targetPCHi)<<8 | uint16(targetPCLo))
    targetPC := offsetFrom + int(offset)

    // Push break completion
    vm.completionStack = append(vm.completionStack, Completion{
        Type:     CompletionBreak,
        TargetPC: targetPC,
    })

    frame.ip = ip
    continue
```

##### OpPushContinue

Similar to OpPushBreak.

##### OpHandlePending (Enhanced)

```go
case OpHandlePending:
    frame.ip = ip

    // Check if there's a completion on the stack
    if len(vm.completionStack) > 0 {
        completion := vm.completionStack[len(vm.completionStack)-1]
        vm.completionStack = vm.completionStack[:len(vm.completionStack)-1]

        switch completion.Type {
        case CompletionBreak, CompletionContinue:
            // Jump to the target PC
            ip = completion.TargetPC
            frame.ip = ip
            continue

        case CompletionReturn:
            // Handle return (existing logic)
            result := vm.pendingValue
            vm.pendingAction = ActionNone
            vm.pendingValue = Undefined
            // ... existing return logic ...

        case CompletionThrow:
            // Handle throw (existing logic)
            vm.currentException = vm.pendingValue
            vm.pendingAction = ActionNone
            vm.pendingValue = Undefined
            vm.unwinding = true
        }
    }

    // Also handle legacy pendingAction for return/throw
    // (existing OpHandlePending logic)
```

### Generated Bytecode Examples

#### Example 1: Simple Break

```javascript
while (c < 2) {
    try {
        c++;
        break;
    } finally {
        fin = 1;
    }
}
```

Bytecode:
```
loop_start:                           // PC 0
  c < 2 → R1
  OpJumpIfFalse R1, @loop_exit        // PC 10
try_start:                            // PC 15
  c++
  OpPushBreak @loop_exit_placeholder  // PC 30: stores loop_exit PC
  OpJump @finally                     // PC 33: jump to finally
try_end:
finally:                              // PC 40
  fin = 1
  OpHandlePending                     // PC 50: checks completion stack
  // Normal exit continues here
  OpJump @loop_start                  // PC 51: normal loop
loop_exit:                            // PC 60
```

When break executes:
1. OpPushBreak pushes {Type: CompletionBreak, TargetPC: 60}
2. OpJump goes to finally (PC 40)
3. Finally executes
4. OpHandlePending pops completion, jumps to 60
5. Loop exits

When normal loop iteration:
1. No OpPushBreak, no completion pushed
2. Normal exit jumps to finally (PC 40)
3. Finally executes
4. OpHandlePending finds no completion, continues to PC 51
5. Loop continues

#### Example 2: Break and Continue

```javascript
while (i < 3) {
    try {
        i++;
        if (i === 1) continue;
        if (i === 2) break;
    } finally {
        fin++;
    }
}
```

Bytecode:
```
loop_start:                              // PC 0
  i < 3 → R1
  OpJumpIfFalse R1, @loop_exit           // PC 10
try_start:                               // PC 15
  i++
  if (i === 1):
    OpPushContinue @loop_start_placeholder  // PC 30
    OpJump @finally                         // PC 33
  if (i === 2):
    OpPushBreak @loop_exit_placeholder      // PC 40
    OpJump @finally                         // PC 43
try_end:
  OpJump @finally                           // Normal exit
finally:                                    // PC 50
  fin++
  OpHandlePending                           // PC 60
  OpJump @loop_start                        // PC 61: for normal iteration
loop_exit:                                  // PC 70
```

When i===1 (continue):
1. OpPushContinue pushes {Type: CompletionContinue, TargetPC: 0}
2. Jump to finally
3. OpHandlePending pops completion, jumps to 0
4. Loop continues from start

When i===2 (break):
1. OpPushBreak pushes {Type: CompletionBreak, TargetPC: 70}
2. Jump to finally
3. OpHandlePending pops completion, jumps to 70
4. Loop exits

When i===3 (normal):
1. No completion pushed
2. Normal exit jumps to finally
3. OpHandlePending finds no completion, continues to PC 61
4. Loop continues

### Performance Considerations

1. **Completion Stack Overhead**:
   - Stack operations are fast (slice append/pop)
   - Only allocates when break/continue in try-finally (rare case)
   - No overhead for normal control flow

2. **Opcode Count**:
   - 2 extra opcodes (OpPushBreak, OpPushContinue)
   - Break/continue in try-finally: +1 opcode overhead vs normal
   - Normal break/continue: zero overhead (same as before)

3. **VM State**:
   - +24 bytes per active completion (Type(8) + Value(16) + TargetPC(8))
   - Typically 0-1 completions on stack
   - Stack cleared after finally

### Migration Path

1. Add Completion struct and completionStack to VM
2. Add OpPushBreak/OpPushContinue opcodes
3. Modify compiler to emit new opcodes when in finally context
4. Enhance OpHandlePending to check completion stack
5. Update loop patching to handle new opcodes
6. Test with existing test suite (should have zero regressions)
7. Add tests for break/continue in finally

### Testing Strategy

Key test cases:
1. Simple break in try-finally inside loop
2. Simple continue in try-finally inside loop
3. Both break and continue in same try (conditional)
4. Normal exit from try (no break/continue)
5. Nested loops with break to outer loop
6. Break in loop inside try block (should work normally)
7. Multiple levels of nested try-finally with breaks

### Alternative Considered: Static Jump Tables

Instead of runtime completion stack, emit a jump table after finally:

```
finally:
  ...
  OpHandlePending
jump_table:
  OpJump @target1  // For break from statement 1
  OpJump @target2  // For continue from statement 2
  ...
```

Then OpPushBreak stores an INDEX, not a PC.

**Rejected because**:
- More complex to implement (need to track indices)
- Less flexible (fixed jump table size)
- Doesn't handle dynamic cases well
- Completion stack is cleaner and more extensible

### Summary

The completion stack approach is:
- **Correct**: Matches ECMAScript semantics
- **Performant**: Minimal overhead, zero cost for normal cases
- **Extensible**: Can handle future cases (labeled breaks, etc.)
- **Clean**: Clear separation of concerns between compiler and VM

This is the proper way to implement try-finally with break/continue in a production-quality runtime.

# Async/Await Implementation Design

This document outlines the comprehensive design for implementing async/await in Paserati, building on the existing generator infrastructure while adding a pluggable async runtime system.

## Table of Contents

1. [Overview](#overview)
2. [Design Philosophy](#design-philosophy)
3. [Architecture](#architecture)
4. [Pluggable Async Runtime](#pluggable-async-runtime)
5. [Promise Implementation](#promise-implementation)
6. [Async Function Transformation](#async-function-transformation)
7. [Top-Level Await](#top-level-await)
8. [Implementation Phases](#implementation-phases)
9. [Performance Considerations](#performance-considerations)
10. [ECMAScript Compliance](#ecmascript-compliance)

---

## Overview

### Goals

1. **Modern TypeScript** - Full async/await support with proper typing
2. **Pluggable Runtime** - Async execution engine is injectable (event loop, Go runtime, custom)
3. **Top-Level Await** - Support async at module scope from day one
4. **Zero Overhead** - No cost for synchronous code
5. **ECMAScript Compliant** - Full ES2017+ async/await + ES2022 top-level await
6. **Generator Synergy** - Build on existing generator infrastructure
7. **Async Generators** - Support `async function*` and `for await...of`

### Non-Goals

- Built-in event loop (pluggable instead)
- Web APIs (fetch, setTimeout, etc.) - provided by runtime
- Browser compatibility layers

---

## Design Philosophy

### Separation of Concerns

**Core Engine** (Paserati)
- Compiles async/await to state machine bytecode
- Manages Promise objects and their state
- Provides hooks for async runtime integration
- Type system for async types

**Async Runtime** (Pluggable)
- Schedules microtasks
- Manages event loop (if applicable)
- Provides I/O primitives
- Integrates with host environment (Go, browser, Node.js-compat, etc.)

### Why Pluggable?

Different deployment scenarios need different async models:

1. **Server/CLI**: Go-based runtime using goroutines and channels
2. **Embedded**: Custom scheduling for embedded systems
3. **Node.js Compat**: libuv-style event loop
4. **Testing**: Deterministic synchronous execution
5. **Edge**: Cloudflare Workers-style execution

---

## Architecture

### High-Level Flow

```
TypeScript Source
      ↓
  [Lexer/Parser]
      ↓
  [Type Checker] ← Promise<T>, async/await types
      ↓
  [Compiler] ← Transform async → generator-like state machine
      ↓
  [VM Bytecode]
      ↓
  [VM Execution] ←→ [Async Runtime Interface]
      ↓                    ↓
  Promise Objects    Schedule Microtasks
```

### Key Insight: Async as Enhanced Generators

Async functions are **generators that yield Promises**:

```typescript
// Conceptual transformation
async function foo() {
    const x = await bar();
    return x + 1;
}

// Desugars to (conceptually)
function foo() {
    return new Promise((resolve, reject) => {
        const gen = function*() {
            const x = yield bar();  // bar() returns Promise
            return x + 1;
        }();

        function step(value) {
            const result = gen.next(value);
            if (result.done) {
                resolve(result.value);
            } else {
                Promise.resolve(result.value).then(step, reject);
            }
        }
        step(undefined);
    });
}
```

---

## Pluggable Async Runtime

### Runtime Interface

```go
// pkg/runtime/async.go
package runtime

import "paserati/pkg/vm"

// AsyncRuntime provides the async execution environment
type AsyncRuntime interface {
    // ScheduleMicrotask queues a callback to run after current task completes
    ScheduleMicrotask(callback func())

    // ScheduleTask queues a callback as a new task
    ScheduleTask(callback func())

    // RunUntilIdle executes all pending microtasks and returns
    // Returns true if more work was done
    RunUntilIdle() bool

    // CreateTimer schedules a callback after delay (optional)
    CreateTimer(delay int64, callback func()) TimerHandle

    // CancelTimer cancels a pending timer (optional)
    CancelTimer(handle TimerHandle)
}

// TimerHandle is an opaque handle to a scheduled timer
type TimerHandle interface{}

// DefaultAsyncRuntime is a simple Go-based runtime
type DefaultAsyncRuntime struct {
    microtasks []func()
    tasks      []func()
    mu         sync.Mutex
}

func (rt *DefaultAsyncRuntime) ScheduleMicrotask(callback func()) {
    rt.mu.Lock()
    defer rt.mu.Unlock()
    rt.microtasks = append(rt.microtasks, callback)
}

func (rt *DefaultAsyncRuntime) RunUntilIdle() bool {
    rt.mu.Lock()
    tasks := rt.microtasks
    rt.microtasks = nil
    rt.mu.Unlock()

    if len(tasks) == 0 {
        return false
    }

    for _, task := range tasks {
        task()
    }
    return true
}

// ... task scheduling, timers
```

### Integration with VM

```go
// pkg/vm/vm.go
type VM struct {
    // ... existing fields

    // Async runtime (pluggable)
    asyncRuntime runtime.AsyncRuntime
}

// SetAsyncRuntime sets the async execution runtime
func (vm *VM) SetAsyncRuntime(rt runtime.AsyncRuntime) {
    vm.asyncRuntime = rt
}

// GetAsyncRuntime returns the current async runtime (or default)
func (vm *VM) GetAsyncRuntime() runtime.AsyncRuntime {
    if vm.asyncRuntime == nil {
        vm.asyncRuntime = runtime.NewDefaultAsyncRuntime()
    }
    return vm.asyncRuntime
}

// DrainMicrotasks runs all pending microtasks
func (vm *VM) DrainMicrotasks() {
    rt := vm.GetAsyncRuntime()
    for rt.RunUntilIdle() {
        // Keep draining until no more work
    }
}
```

### Driver Integration

```go
// pkg/driver/driver.go
type Paserati struct {
    // ... existing fields
    asyncRuntime runtime.AsyncRuntime
}

// SetAsyncRuntime configures the async runtime
func (p *Paserati) SetAsyncRuntime(rt runtime.AsyncRuntime) {
    p.asyncRuntime = rt
    p.vmInstance.SetAsyncRuntime(rt)
}

// RunString modified to drain microtasks after execution
func (p *Paserati) RunString(sourceCode string) (vm.Value, []errors.PaseratiError) {
    // ... existing compilation

    // Execute the chunk
    finalValue, runtimeErrs := p.vmInstance.Interpret(chunk)

    // Drain microtasks for async functions
    p.vmInstance.DrainMicrotasks()

    return finalValue, runtimeErrs
}
```

---

## Promise Implementation

### Promise States

```go
// pkg/vm/promise.go
package vm

type PromiseState int

const (
    PromisePending PromiseState = iota
    PromiseFulfilled
    PromiseRejected
)

type PromiseObject struct {
    Object
    State       PromiseState
    Result      Value      // Fulfillment value or rejection reason

    // Reaction records for .then() callbacks
    fulfillReactions []PromiseReaction
    rejectReactions  []PromiseReaction
}

type PromiseReaction struct {
    Handler  Value  // Function to call
    Resolve  func(Value)  // Resolve next promise in chain
    Reject   func(Value)  // Reject next promise in chain
}
```

### Promise Methods

```go
// NewPromise creates a new Promise with an executor function
func NewPromise(vm *VM, executor Value) (Value, error) {
    promise := &PromiseObject{
        State:            PromisePending,
        Result:           Undefined,
        fulfillReactions: []PromiseReaction{},
        rejectReactions:  []PromiseReaction{},
    }

    // Set up prototype chain
    promise.Object = *NewObject(vm.PromisePrototype).AsPlainObject()

    promiseVal := NewValueFromPromise(promise)

    // Create resolve/reject callbacks
    resolve := NewNativeFunction(1, false, "resolve", func(args []Value) (Value, error) {
        if promise.State != PromisePending {
            return Undefined, nil
        }
        value := Undefined
        if len(args) > 0 {
            value = args[0]
        }
        vm.resolvePromise(promise, value)
        return Undefined, nil
    })

    reject := NewNativeFunction(1, false, "reject", func(args []Value) (Value, error) {
        if promise.State != PromisePending {
            return Undefined, nil
        }
        reason := Undefined
        if len(args) > 0 {
            reason = args[0]
        }
        vm.rejectPromise(promise, reason)
        return Undefined, nil
    })

    // Call executor(resolve, reject)
    _, err := vm.Call(executor, Undefined, []Value{resolve, reject})
    if err != nil {
        vm.rejectPromise(promise, NewString(err.Error()))
    }

    return promiseVal, nil
}

func (vm *VM) resolvePromise(promise *PromiseObject, value Value) {
    if promise.State != PromisePending {
        return
    }

    // Handle promise resolution with thenable chaining
    if value.Type() == TypePromise {
        otherPromise := value.AsPromise()
        if otherPromise.State == PromiseFulfilled {
            value = otherPromise.Result
        } else if otherPromise.State == PromiseRejected {
            vm.rejectPromise(promise, otherPromise.Result)
            return
        } else {
            // Chain to pending promise
            vm.promiseThen(value,
                func(v Value) { vm.resolvePromise(promise, v) },
                func(r Value) { vm.rejectPromise(promise, r) })
            return
        }
    }

    promise.State = PromiseFulfilled
    promise.Result = value

    // Schedule fulfillment reactions as microtasks
    rt := vm.GetAsyncRuntime()
    for _, reaction := range promise.fulfillReactions {
        reaction := reaction  // Capture
        rt.ScheduleMicrotask(func() {
            result, err := vm.Call(reaction.Handler, Undefined, []Value{value})
            if err != nil {
                reaction.Reject(NewString(err.Error()))
            } else {
                reaction.Resolve(result)
            }
        })
    }
    promise.fulfillReactions = nil
}

func (vm *VM) rejectPromise(promise *PromiseObject, reason Value) {
    if promise.State != PromisePending {
        return
    }

    promise.State = PromiseRejected
    promise.Result = reason

    // Schedule rejection reactions as microtasks
    rt := vm.GetAsyncRuntime()
    for _, reaction := range promise.rejectReactions {
        reaction := reaction  // Capture
        rt.ScheduleMicrotask(func() {
            result, err := vm.Call(reaction.Handler, Undefined, []Value{reason})
            if err != nil {
                reaction.Reject(NewString(err.Error()))
            } else {
                reaction.Resolve(result)
            }
        })
    }
    promise.rejectReactions = nil
}
```

### Promise.prototype.then()

```go
// pkg/builtins/promise_init.go
func setupPromisePrototype(vm *VM) {
    promiseProto := NewObject(vm.ObjectPrototype).AsPlainObject()

    // Promise.prototype.then(onFulfilled, onRejected)
    promiseProto.SetOwn("then", NewNativeFunction(2, false, "then", func(args []Value) (Value, error) {
        thisPromise := vm.GetThis().AsPromise()
        if thisPromise == nil {
            return Undefined, fmt.Errorf("TypeError: Promise.prototype.then called on non-Promise")
        }

        onFulfilled := Undefined
        onRejected := Undefined
        if len(args) > 0 {
            onFulfilled = args[0]
        }
        if len(args) > 1 {
            onRejected = args[1]
        }

        // Create new promise for chaining
        var chainedPromise *PromiseObject
        promiseVal, _ := NewPromise(vm, NewNativeFunction(2, false, "executor", func(execArgs []Value) (Value, error) {
            resolve := execArgs[0]
            reject := execArgs[1]

            // Register reactions
            if onFulfilled.IsCallable() {
                thisPromise.fulfillReactions = append(thisPromise.fulfillReactions, PromiseReaction{
                    Handler: onFulfilled,
                    Resolve: func(v Value) { vm.Call(resolve, Undefined, []Value{v}) },
                    Reject:  func(r Value) { vm.Call(reject, Undefined, []Value{r}) },
                })
            }

            if onRejected.IsCallable() {
                thisPromise.rejectReactions = append(thisPromise.rejectReactions, PromiseReaction{
                    Handler: onRejected,
                    Resolve: func(v Value) { vm.Call(resolve, Undefined, []Value{v}) },
                    Reject:  func(r Value) { vm.Call(reject, Undefined, []Value{r}) },
                })
            }

            // If already settled, schedule reaction immediately
            if thisPromise.State == PromiseFulfilled && onFulfilled.IsCallable() {
                vm.GetAsyncRuntime().ScheduleMicrotask(func() {
                    result, err := vm.Call(onFulfilled, Undefined, []Value{thisPromise.Result})
                    if err != nil {
                        vm.Call(reject, Undefined, []Value{NewString(err.Error())})
                    } else {
                        vm.Call(resolve, Undefined, []Value{result})
                    }
                })
            } else if thisPromise.State == PromiseRejected && onRejected.IsCallable() {
                vm.GetAsyncRuntime().ScheduleMicrotask(func() {
                    result, err := vm.Call(onRejected, Undefined, []Value{thisPromise.Result})
                    if err != nil {
                        vm.Call(reject, Undefined, []Value{NewString(err.Error())})
                    } else {
                        vm.Call(resolve, Undefined, []Value{result})
                    }
                })
            }

            return Undefined, nil
        }))

        chainedPromise = promiseVal.AsPromise()
        return promiseVal, nil
    }))

    // Promise.prototype.catch(onRejected)
    promiseProto.SetOwn("catch", NewNativeFunction(1, false, "catch", func(args []Value) (Value, error) {
        onRejected := Undefined
        if len(args) > 0 {
            onRejected = args[0]
        }
        return vm.Call(vm.GetThis().GetProperty("then"), vm.GetThis(), []Value{Undefined, onRejected})
    }))

    vm.PromisePrototype = NewValueFromPlainObject(promiseProto)
}
```

---

## Async Function Transformation

### Compilation Strategy

Async functions compile to generator-like state machines that yield Promises.

#### Example Transformation

```typescript
// Source
async function fetchData(url: string): Promise<Data> {
    const response = await fetch(url);
    const data = await response.json();
    return data;
}

// Conceptual bytecode (simplified)
function fetchData$async(url) {
    return new Promise((resolve, reject) => {
        const $state = {
            pc: 0,
            locals: { url, response: undefined, data: undefined }
        };

        function $resume(value, isError) {
            try {
                switch ($state.pc) {
                    case 0:  // Start
                        const promise1 = fetch($state.locals.url);
                        $state.pc = 1;
                        promise1.then(v => $resume(v, false), e => $resume(e, true));
                        return;

                    case 1:  // After first await
                        if (isError) throw value;
                        $state.locals.response = value;
                        const promise2 = $state.locals.response.json();
                        $state.pc = 2;
                        promise2.then(v => $resume(v, false), e => $resume(e, true));
                        return;

                    case 2:  // After second await
                        if (isError) throw value;
                        $state.locals.data = value;
                        resolve($state.locals.data);
                        return;
                }
            } catch (e) {
                reject(e);
            }
        }

        $resume(undefined, false);
    });
}
```

### Compiler Implementation

```go
// pkg/compiler/compile_async.go
package compiler

func (c *Compiler) compileAsyncFunction(node *parser.FunctionLiteral) int {
    // Compile body as generator-like state machine
    funcReg := c.allocateRegister()
    defer c.freeRegister(funcReg)

    // Create wrapper function that returns Promise
    wrapperFunc := c.createAsyncWrapper(node)

    // Store compiled function
    funcIdx := c.addConstant(wrapperFunc)
    c.emit(OpLoadConst, funcReg, uint16(funcIdx)>>8, uint16(funcIdx)&0xFF)

    return funcReg
}

func (c *Compiler) createAsyncWrapper(node *parser.FunctionLiteral) Value {
    // Create a function that:
    // 1. Creates a Promise
    // 2. Sets up state machine executor
    // 3. Returns the Promise

    chunk := NewChunk(node.Name.Value + "$async")

    // Emit: Create Promise executor
    c.emitAsyncStateMachine(chunk, node)

    return NewFunction(chunk, node.Parameters, false /* not generator */)
}

func (c *Compiler) compileAwaitExpression(node *parser.AwaitExpression) int {
    // Compile expression to await
    promiseReg := c.compileExpression(node.Argument)
    defer c.freeRegister(promiseReg)

    resultReg := c.allocateRegister()

    // Emit OpAwait - yields control until promise resolves
    // Similar to OpYield but integrates with Promise resolution
    c.emit(OpAwait, resultReg, promiseReg)

    return resultReg
}
```

### New Opcodes

```go
// pkg/vm/bytecode.go
const (
    // ... existing opcodes

    OpCreateAsyncFunction OpCode = 95  // Create async function (returns Promise)
    OpAwait               OpCode = 96  // Suspend until promise resolves
    OpResumeAsync         OpCode = 97  // Internal: resume async function
)
```

### VM Execution

```go
// pkg/vm/vm.go
func (vm *VM) run() (InterpretResult, Value) {
    // ... existing loop

    case OpAwait:
        destReg := code[ip]
        promiseReg := code[ip+1]
        ip += 2

        promiseVal := registers[promiseReg]

        // Get current async function context
        asyncFrame := vm.getCurrentAsyncFrame()
        if asyncFrame == nil {
            status := vm.runtimeError("await used outside async function")
            return status, Undefined
        }

        // Save execution state
        asyncFrame.State = AsyncSuspendedAwait
        asyncFrame.ResumeIP = ip
        asyncFrame.ResumeReg = destReg
        asyncFrame.Registers = make([]Value, len(registers))
        copy(asyncFrame.Registers, registers)

        // Attach promise handlers
        promise := promiseVal.AsPromise()
        if promise == nil {
            // Not a promise - wrap in resolved promise
            promise = vm.createResolvedPromise(promiseVal)
        }

        // Schedule resumption when promise settles
        vm.promiseThen(promiseVal,
            func(value Value) {
                // Resume with fulfilled value
                vm.resumeAsyncFunction(asyncFrame, value, false)
            },
            func(reason Value) {
                // Resume with rejection
                vm.resumeAsyncFunction(asyncFrame, reason, true)
            })

        // Return to caller - promise will resume us
        return InterpretOK, Undefined
}

func (vm *VM) resumeAsyncFunction(frame *AsyncFrame, value Value, isError bool) {
    // Restore frame state
    vm.frameCount++
    currentFrame := &vm.frames[vm.frameCount-1]
    currentFrame.closure = frame.Closure
    currentFrame.ip = frame.ResumeIP
    currentFrame.registers = frame.Registers

    if isError {
        // Throw the error into the async function
        vm.throwException(value)
    } else {
        // Store resolved value in destination register
        currentFrame.registers[frame.ResumeReg] = value
    }

    // Continue execution
    vm.run()
}
```

---

## Top-Level Await

Top-level await allows `await` at module scope without wrapping in async function.

### Design

Modules with top-level await compile to async functions that return Promises.

```typescript
// source.ts
const data = await fetchData();
export const processed = process(data);

// Compiles to (conceptually)
export default async function $module() {
    const data = await fetchData();
    export const processed = process(data);
}

// Module loader
const modulePromise = $module();
await modulePromise;  // Wait for module initialization
```

### Module Loading

```go
// pkg/modules/loader.go
type ModuleRecord struct {
    // ... existing fields

    IsAsync         bool         // True if module uses top-level await
    InitPromise     vm.Value     // Promise for async module initialization
}

func (ml *ModuleLoader) LoadModule(specifier string, fromPath string) (ModuleRecord, error) {
    // ... existing loading

    // Check if module uses top-level await
    hasTopLevelAwait := ml.detectTopLevelAwait(ast)

    if hasTopLevelAwait {
        // Compile as async module
        chunk := compiler.CompileAsAsyncModule(ast)

        // Execute to get initialization promise
        initPromise, err := vm.Interpret(chunk)
        if err != nil {
            return nil, err
        }

        record.IsAsync = true
        record.InitPromise = initPromise

        // Block until module initializes
        vm.awaitPromise(initPromise)
    }

    return record, nil
}
```

### Compilation

```go
// pkg/compiler/compile_module.go
func (c *Compiler) CompileAsAsyncModule(program *parser.Program) *Chunk {
    // Wrap entire module in async function
    chunk := NewChunk("<module>")

    // Emit: Create Promise
    promiseReg := c.allocateRegister()
    c.emitCreatePromise(promiseReg)

    // Compile module body as async state machine
    c.compileModuleBody(program, promiseReg)

    // Return promise
    c.emit(OpReturn, promiseReg)

    return chunk
}
```

---

## Implementation Phases

### Phase 1: Promise Foundation ✅ COMPLETED

**Goals**: Basic Promise object and runtime interface

- [x] Define `AsyncRuntime` interface in `pkg/runtime/async.go`
- [x] Implement `DefaultAsyncRuntime` with microtask queue
- [x] Create `PromiseObject` in `pkg/vm/promise.go`
- [x] Implement `Promise.prototype.then/catch/finally`
- [x] Add `Promise` type to type system (integrated into VM value types)
- [x] Wire `AsyncRuntime` into VM and Driver
- [x] Test: Manual promise creation and chaining

**Implementation Details**:

**Files Created**:
- `pkg/runtime/async.go` - Pluggable async runtime interface with `DefaultAsyncRuntime`
- `pkg/vm/promise.go` - Complete Promise implementation with state machine
- `pkg/vm/async_runtime.go` - VM integration methods (SetAsyncRuntime, GetAsyncRuntime, DrainMicrotasks)
- `pkg/builtins/promise_init.go` - Promise builtin initializer following Generator pattern
- `tests/scripts/promise_basic.ts` - Smoke test for Promise constructor
- `tests/scripts/promise_resolve_then.ts` - Smoke test for Promise.resolve() and .then()

**Files Modified**:
- `pkg/vm/vm.go` - Added `asyncRuntime` field and `PromisePrototype`
- `pkg/vm/value.go` - Added `TypePromise`, `AsPromise()`, ToString case
- `pkg/vm/property_helpers.go` - Added TypePromise prototype chain lookup (critical fix)
- `pkg/builtins/standard.go` - Registered PromiseInitializer
- `pkg/driver/driver.go` - Added DrainMicrotasks() to RunString() and RunCode()

**Key Features Working**:
- Promise creation with executor function
- Promise.resolve() and Promise.reject() static methods
- Promise.prototype.then/catch/finally methods
- Microtask scheduling via pluggable async runtime
- Automatic microtask draining after script execution
- Promise chaining with proper value propagation
- Already-settled promise handling

**Deliverables** (Verified Working):
```typescript
// Promise constructor
const p = new Promise((resolve) => resolve(42));
p.then(x => console.log(x));  // Prints 42

// Promise.resolve() with chaining
const p2 = Promise.resolve(42);
p2.then(x => console.log('Then:', x));  // Prints "Then: 42"

// Microtask execution verified
const p3 = Promise.resolve(42);
p3.then(x => console.log('Then:', x));
console.log('End');  // Output: "End" then "Then: 42"
```

**Technical Highlights**:
- Pluggable runtime design allows custom async implementations
- DefaultAsyncRuntime uses mutex-protected microtask queue
- Promise state machine: Pending → Fulfilled/Rejected with proper transitions
- Promise reactions stored as closures with resolve/reject callbacks
- Microtasks scheduled via AsyncRuntime.ScheduleMicrotask()
- VM automatically drains microtasks after synchronous execution completes
- Promise objects inherit from Promise.prototype via prototype chain

### Phase 2: Async Function Transformation ✅ COMPLETED

**Goals**: Compile async functions to promise-returning state machines

- [x] Add `async` keyword to lexer/parser
- [x] Parse `async function` declarations and expressions
- [x] Parse `async` arrow functions
- [x] Implement async function type in checker (`Function → Promise<T>`)
- [x] Add `OpAwait` opcode (95)
- [x] Compiler: Generate OpAwait bytecode for await expressions
- [x] VM: Async functions return Promise immediately
- [x] VM: OpAwait suspension/resumption using SuspendedFrame (like generators)
- [x] Attach promise settlement handlers to resume execution
- [x] Test: Basic async/await with already-resolved promises
- [x] Test: Async/await with pending promises

**Implementation Notes**:

**Architectural Decision**: Reuse GeneratorFrame → Rename to SuspendedFrame
- Both generators and async functions use identical suspension mechanism
- Same saved state: PC, registers, output register
- Generator: Explicit resumption via `.next()`
- Async: Automatic resumption via promise settlement handlers

**Implementation Details**:
1. **executeAsyncFunction** (pkg/vm/async.go): Creates Promise, schedules async function body as microtask
2. **executeAsyncFunctionBody** (pkg/vm/async.go): Uses sentinel frame approach like generators, sets `frame.promiseObj` for OpAwait access
3. **OpAwait** (pkg/vm/vm.go): Handles three cases:
   - Already fulfilled: Store result and continue
   - Already rejected: Throw error
   - Pending: Save SuspendedFrame, attach handlers, suspend
4. **resumeAsyncFunction** (pkg/vm/vm.go): Restores saved frame, resumes execution from await point
5. **resumeAsyncFunctionWithException** (pkg/vm/vm.go): Resumes and throws exception at await point

**Current Status**: ✅ **FULLY WORKING**
- ✅ Lexer, Parser, Type Checker complete
- ✅ OpAwait compilation working
- ✅ Async functions return Promise on call via executeAsyncFunction
- ✅ OpAwait VM execution with full suspension/resumption
- ✅ Promise settlement handlers trigger async resumption
- ✅ Multiple awaits in sequence working
- ✅ Try/catch exception handling in async functions (**NEW**)
- ✅ Type inference: async functions properly typed as `() => Promise<T>` (**NEW**)
- ✅ Smoke tests passing (async_await_basic, async_await_multiple, async_await_pending, async_try_catch)

**Recent Fixes**:
1. **Type Inference Fix** (pkg/checker/checker.go, pkg/checker/function.go):
   - Added `IsAsync` field to hoisting context
   - Updated `createPromiseType()` to use `types.PromiseGeneric` instead of circular ObjectType
   - Promise wrapping now happens in Pass 3 alongside generators

2. **Try/Catch Fix** (pkg/vm/exceptions.go):
   - Fixed `unwindException()` to check for exception handlers BEFORE checking `isDirectCall`
   - Async functions and generators can now properly catch exceptions in try/catch blocks
   - Also fixed try/catch in generators as a side effect

**Verified Working**:
```typescript
async function test() {
    const x = await Promise.resolve(42);
    return x + 1;
}
test().then(console.log);  // Prints 43

// Multiple awaits
async function multipleAwaits() {
    const a = await Promise.resolve(10);
    const b = await Promise.resolve(20);
    const c = await Promise.resolve(30);
    return a + b + c;
}
multipleAwaits().then(console.log); // Prints 60

// Pending promises
let resolve: (value: number) => void;
const pending = new Promise<number>(r => { resolve = r; });
async function testPending() {
    const result = await pending;
    return result + 100;
}
testPending().then(console.log); // Will print 142 after resolve(42)
resolve(42);
```

### Phase 3: Advanced Await Expressions ✅ COMPLETED

**Goals**: Full await expression support with error handling

- [x] Parse `await` expressions in async contexts
- [x] Implement `AwaitExpression` compilation
- [x] Handle promise rejection in await
- [x] Try/catch integration with async functions
- [x] Multiple awaits in sequence
- [x] Parallel await (Promise.all/race/allSettled)
- [x] Test: Error handling, nested awaits

**Current Status**: ✅ **FULLY WORKING**
- ✅ Basic await expressions working
- ✅ Sequential awaits working
- ✅ Try/catch error handling working
- ✅ Parallel patterns working (Promise.all/race/allSettled)

**Verified Working**:
```typescript
// Error handling
async function fetchData() {
    try {
        const x = await Promise.resolve("data1");
        const y = await Promise.resolve("data2");
        return [x, y];
    } catch (e) {
        console.error(e);
        return null;
    }
}

// Parallel patterns
async function parallel() {
    const results = await Promise.all([
        Promise.resolve(1),
        Promise.resolve(2),
        Promise.resolve(3)
    ]);
    console.log(results); // [1, 2, 3]
}
```

### Phase 4: Top-Level Await (Week 5-6)

**Goals**: Module-level await support

- [ ] Detect top-level await in modules
- [ ] Compile modules with TLA as async functions
- [ ] Module loader: Handle async module initialization
- [ ] Module dependency graph with async modules
- [ ] Test: Cross-module top-level await

**Deliverables**:
```typescript
// module.ts
const config = await loadConfig();
export { config };

// main.ts
import { config } from './module.ts';
```

### Phase 5: Async Generators (Week 7-8)

**Goals**: `async function*` and `for await...of`

- [ ] Parse `async function*` syntax
- [ ] Implement `AsyncGenerator<T, TReturn, TNext>` type
- [ ] Compile async generators (combination of async + generator state)
- [ ] `for await...of` loop compilation
- [ ] `AsyncIterator` protocol
- [ ] Test: Async iteration, streams

**Deliverables**:
```typescript
async function* asyncRange(n: number) {
    for (let i = 0; i < n; i++) {
        await sleep(100);
        yield i;
    }
}

for await (const x of asyncRange(5)) {
    console.log(x);
}
```

### Phase 6: Optimization & Polish ⏳ IN PROGRESS

**Goals**: Performance and ECMAScript compliance

- [ ] Optimize promise creation (object pooling)
- [ ] Microtask batching
- [ ] Stack trace preservation across awaits
- [x] `Promise.all`, `Promise.race`, `Promise.allSettled` ✅ **COMPLETED**
- [ ] Full ECMAScript Test262 compliance for Promises
- [ ] Benchmark async overhead
- [ ] Documentation and examples

**Recent Completions**:

**Promise Static Methods** (pkg/builtins/promise_init.go):
- ✅ `Promise.all(iterable)`: Waits for all promises to resolve, rejects on first rejection
- ✅ `Promise.race(iterable)`: Settles with first settled promise (fulfill or reject)
- ✅ `Promise.allSettled(iterable)`: Waits for all promises to settle, returns array of result objects

**Helper Methods** (pkg/vm/promise.go):
- ✅ `IterableToArray()`: Converts iterables to arrays (currently supports arrays)
- ✅ `NewArrayFromSlice()`: Creates arrays from Go slices

**Type Definitions** (pkg/builtins/promise_init.go):
- ✅ Added type signatures for all three static methods

**Smoke Tests** (tests/scripts/):
- ✅ `promise_all.ts`: Basic Promise.all with multiple promises
- ✅ `promise_all_mixed.ts`: Promise.all with mixed promises and values
- ✅ `promise_all_reject.ts`: Promise.all rejection behavior
- ✅ `promise_all_empty.ts`: Promise.all with empty array
- ✅ `promise_race.ts`: Basic Promise.race
- ✅ `promise_race_reject.ts`: Promise.race rejection behavior
- ✅ `promise_allsettled.ts`: Promise.allSettled with mixed results

**Verified Working**:
```typescript
// Promise.all
const results = await Promise.all([
    Promise.resolve(1),
    Promise.resolve(2),
    Promise.resolve(3)
]);
console.log(results); // [1, 2, 3]

// Promise.race
const first = await Promise.race([
    Promise.resolve('first'),
    Promise.resolve('second')
]);
console.log(first); // 'first'

// Promise.allSettled
const settled = await Promise.allSettled([
    Promise.resolve(1),
    Promise.reject('error'),
    Promise.resolve(3)
]);
console.log(settled);
// [
//   {status: "fulfilled", value: 1},
//   {status: "rejected", reason: "error"},
//   {status: "fulfilled", value: 3}
// ]
```

---

## Performance Considerations

### Zero-Cost Abstractions

**Synchronous Code**: No async overhead
- Regular functions remain unchanged
- No promise creation for sync paths
- No microtask scheduling

**Async Functions**: Minimal overhead
- State machine in bytecode (not heap allocation per await)
- Promises allocated only when needed
- Microtask queue optimized with batching

### Optimization Strategies

1. **Promise Pooling**: Reuse Promise objects for common patterns
2. **Inline Caching**: Cache promise resolution paths
3. **Fast Path Detection**: Detect already-resolved promises
4. **Microtask Batching**: Process microtasks in batches
5. **Stack Unwinding**: Efficient async stack traces

### Benchmarks

Target performance relative to other engines:

- **V8 (Node.js)**: 80-90% performance
- **QuickJS**: 100-110% (beat on some async patterns)
- **Deno**: 85-95% performance

---

## ECMAScript Compliance

### ES2017 (Async/Await)

- [x] `async function` declarations
- [x] `async` function expressions
- [x] `async` arrow functions
- [x] `await` expressions in async contexts
- [x] Promise integration
- [x] Error propagation (try/catch with await)

### ES2018 (Async Iteration)

- [ ] `async function*` declarations
- [ ] `for await...of` loops
- [ ] `AsyncIterator` protocol
- [ ] `AsyncGenerator` type

### ES2020 (Promise Combinators)

- [ ] `Promise.allSettled()`
- [ ] `Promise.any()`

### ES2022 (Top-Level Await)

- [ ] Module-level `await`
- [ ] Async module initialization
- [ ] Module graph with async dependencies

---

## Type System Integration

### Promise Types

```typescript
// Built-in Promise type
interface Promise<T> {
    then<TResult1 = T, TResult2 = never>(
        onfulfilled?: ((value: T) => TResult1 | PromiseLike<TResult1>) | null,
        onrejected?: ((reason: any) => TResult2 | PromiseLike<TResult2>) | null
    ): Promise<TResult1 | TResult2>;

    catch<TResult = never>(
        onrejected?: ((reason: any) => TResult | PromiseLike<TResult>) | null
    ): Promise<T | TResult>;

    finally(onfinally?: (() => void) | null): Promise<T>;
}

interface PromiseConstructor {
    new <T>(executor: (resolve: (value: T) => void, reject: (reason?: any) => void) => void): Promise<T>;

    resolve<T>(value: T): Promise<T>;
    reject<T = never>(reason?: any): Promise<T>;
    all<T>(values: Iterable<T | PromiseLike<T>>): Promise<T[]>;
    race<T>(values: Iterable<T | PromiseLike<T>>): Promise<T>;
}

declare var Promise: PromiseConstructor;
```

### Async Function Types

```typescript
// Async function type inference
async function foo(): Promise<number> {
    return 42;  // Checker infers Promise<number> from async
}

// Async arrow functions
const bar = async () => 42;  // Type: () => Promise<number>

// Await type narrowing
async function test(p: Promise<number | string>) {
    const x = await p;  // Type: number | string
    if (typeof x === "number") {
        // x is number here
    }
}
```

### Async Generator Types

```typescript
interface AsyncIterator<T, TReturn = any, TNext = undefined> {
    next(...args: [] | [TNext]): Promise<IteratorResult<T, TReturn>>;
    return?(value?: TReturn): Promise<IteratorResult<T, TReturn>>;
    throw?(e?: any): Promise<IteratorResult<T, TReturn>>;
}

interface AsyncGenerator<T = unknown, TReturn = any, TNext = unknown>
    extends AsyncIterator<T, TReturn, TNext> {
    [Symbol.asyncIterator](): AsyncGenerator<T, TReturn, TNext>;
}
```

---

## Testing Strategy

### Unit Tests

- Promise state machine (pending → fulfilled/rejected)
- Microtask scheduling
- Promise chaining
- Error propagation
- Async function compilation
- Await expression execution

### Integration Tests

- Async + generators
- Async + exceptions
- Async + closures
- Async + classes
- Top-level await + modules
- Cross-module async dependencies

### Compliance Tests

- ECMAScript Test262 Promise tests
- ECMAScript Test262 async/await tests
- Custom Paserati async test suite

### Performance Tests

- Async overhead benchmarks
- Promise creation overhead
- Microtask scheduling latency
- Comparison with V8, QuickJS

---

## Example Use Cases

### 1. HTTP Server (Go Runtime)

```typescript
// Using Go-based async runtime
import { serve } from "paserati/http";

const server = serve({ port: 3000 });

for await (const req of server) {
    const data = await fetchData(req.url);
    req.respond({ body: data });
}
```

### 2. CLI Tool with Concurrent Operations

```typescript
async function processFiles(files: string[]) {
    const results = await Promise.all(
        files.map(async (file) => {
            const content = await readFile(file);
            return processContent(content);
        })
    );
    return results;
}
```

### 3. Module with Top-Level Await

```typescript
// config.ts
const response = await fetch("https://config.example.com");
export const config = await response.json();

// main.ts
import { config } from "./config.ts";
console.log(config);
```

---

## Future Extensions

### WebAssembly Integration

Async runtime backed by WASM event loop for browser deployment.

### Structured Concurrency

Explore structured concurrency patterns (a la Swift, Kotlin):

```typescript
async function parallel<T>(tasks: (() => Promise<T>)[]): Promise<T[]> {
    // Automatic cancellation of pending tasks on first error
}
```

### Async Hooks

Runtime hooks for async operation tracking (profiling, debugging):

```typescript
runtime.onAsyncInit(callback);
runtime.onAsyncBefore(callback);
runtime.onAsyncAfter(callback);
```

---

## Success Criteria

- [ ] Full ES2017 async/await support
- [ ] ES2022 top-level await support
- [ ] ES2018 async generators and `for await...of`
- [ ] Pluggable async runtime interface
- [ ] Default Go-based runtime implementation
- [ ] Promise/A+ compliant Promise implementation
- [ ] ECMAScript Test262 compliance (Promises + async/await)
- [ ] Zero overhead for synchronous code
- [ ] < 10% overhead for async functions vs V8
- [ ] Comprehensive documentation and examples
- [ ] Production-ready error handling and stack traces

---

## References

- [ECMAScript 2017 (ES8) - Async Functions](https://tc39.es/ecma262/#sec-async-function-definitions)
- [ECMAScript 2018 (ES9) - Async Iteration](https://tc39.es/ecma262/#sec-asyncgenerator-objects)
- [ECMAScript 2022 (ES13) - Top-Level Await](https://tc39.es/ecma262/#sec-modules)
- [Promises/A+ Specification](https://promisesaplus.com/)
- [V8 Blog: Fast Async Functions](https://v8.dev/blog/fast-async)
- [Generator Implementation (Paserati)](./generators-implementation-plan.md)

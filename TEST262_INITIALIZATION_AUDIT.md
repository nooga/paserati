# Test262 Runner vs Paserati Binary Initialization Audit

## Executive Summary

**Issue**: Test262 harness code fails with "Cannot read property 'join' of undefined" when accessing `Array.prototype.join`, but the same code works perfectly in the paserati binary.

**Root Cause Hypothesis**: The test262 runner uses a different execution path (`CompileProgram` → `InterpretChunk`) that bypasses critical initialization steps that happen in the standard execution path (`RunCode` → `runAsModule`).

---

## Path 1: Paserati Binary Execution (WORKING)

### Initialization Flow

```
main.go:runExpressionWithTypes()
  ↓
driver.NewPaserati()
  ↓
NewPaseratiWithBaseDir(".")
  ↓
initializeBuiltins(paserati)  ← Initializes Array, Object, Function, etc.
  ↓
driver.RunCode(expr, options)
  ↓
runAsModule(sourceCode, program, moduleName)
  ↓
compiler.Compile(program)  ← Compiles the code
  ↓
p.vmInstance.SyncGlobalNames(p.compiler.GetHeapAlloc().GetNameToIndexMap())  ← CRITICAL STEP
  ↓
p.vmInstance.Interpret(chunk)  ← Executes bytecode
```

### Key Details

**File**: `pkg/driver/driver.go`

**Line ~723** in `runAsModule`:
```go
// Sync global names from compiler to VM heap so globalThis property access works
p.vmInstance.SyncGlobalNames(p.compiler.GetHeapAlloc().GetNameToIndexMap())
```

**What happens**:
1. `NewPaserati()` creates VM and calls `initializeBuiltins()` which:
   - Creates Array, Object, Function, etc. as runtime values
   - Stores them in `vm.heap` with indices
   - Creates name-to-index mappings in the heap
2. `RunCode()` parses and compiles
3. **`runAsModule()` calls `SyncGlobalNames()`** which syncs the compiler's name mappings to the VM heap
4. When JavaScript code accesses `Array.prototype.join`:
   - OpGetGlobal looks up "Array" in `vm.heap.nameToIndex` map
   - Finds the index, retrieves the Array constructor from heap
   - OpGetProp accesses `.prototype` property on the constructor
   - OpGetProp accesses `.join` property on the prototype

---

## Path 2: Test262 Runner Execution (BROKEN)

### Initialization Flow

```
main.go:runSingleTest()
  ↓
createTest262Paserati()
  ↓
driver.NewPaseratiWithInitializers(getTest262EnabledInitializers())
  ↓
initializeBuiltinsWithCustom(paserati, customInitializers)  ← Initializes builtins
  ↓
paserati.SetIgnoreTypeErrors(true)
  ↓
[Concatenate sta.js + assert.js + propertyHelper.js + test code]
  ↓
lexer.NewLexer(sourceWithIncludes)
parser.NewParser(lx)
prog, parseErrs := p.ParseProgram()
  ↓
paserati.CompileProgram(prog)  ← Direct compiler.Compile() call
  ↓
paserati.SyncGlobalNamesFromCompiler()  ← Added recently, calls SyncGlobalNames
  ↓
paserati.InterpretChunk(chunk)  ← Executes bytecode
```

### Key Details

**File**: `cmd/paserati-test262/main.go`

**Lines 546-561**:
```go
chunk, compileErrs := paserati.CompileProgram(prog)
if len(compileErrs) > 0 {
    // error handling
}

// Sync global names from compiler to VM so globalThis property access works
paserati.SyncGlobalNamesFromCompiler()

// Execute compiled chunk
_, runtimeErrs := paserati.InterpretChunk(chunk)
```

**What's different**:
1. Uses `CompileProgram()` directly (bypasses `runAsModule()`)
2. **DOES** call `SyncGlobalNamesFromCompiler()` (we added this)
3. Uses `InterpretChunk()` directly (bypasses module setup)

---

## Critical Difference Analysis

### Compiler State Difference

**Hypothesis 1: Compiler Not Initialized with Heap Allocator**

Looking at `CompileProgram`:
```go
func (p *Paserati) CompileProgram(program *parser.Program) (*vm.Chunk, []errors.PaseratiError) {
	p.compiler.SetIgnoreTypeErrors(p.ignoreTypeErrors)
	return p.compiler.Compile(program)
}
```

This is a simple passthrough. The compiler should already have the heap allocator set during `NewPaseratiWithInitializers()`.

**Verification needed**: Check if `p.compiler.heapAlloc` is properly set in test262 path.

---

### Checker Mode Difference

**Hypothesis 2: Checker Not in Module Mode**

In `runAsModule` (working path):
```go
p.checker.EnableModuleMode(moduleName, p.moduleLoader)
p.compiler.EnableModuleMode(moduleName, p.moduleLoader)
```

In test262 runner (broken path):
```go
// NO call to EnableModuleMode before CompileProgram!
```

**BUT**: In `NewPaseratiWithInitializersAndBaseDir` (line 166):
```go
// Enable module mode for the main checker by default for consistent type checking
typeChecker.EnableModuleMode("", moduleLoader)
```

So the checker IS in module mode by default.

**Verification needed**: Check if test262 runner disables module mode somehow.

---

### Heap Allocator Name Mapping

**Hypothesis 3: Compiler's HeapAlloc Not Tracking User Globals**

When `runAsModule` compiles code, the compiler records new global variables (like the ones defined in harness files) in its HeapAlloc's nameToIndex map. Then `SyncGlobalNames` copies this to the VM heap.

When test262 runner uses `CompileProgram`, the compiler should also record globals in HeapAlloc. We DO call `SyncGlobalNamesFromCompiler()` after compilation.

**BUT**: There's a subtle difference!

In `runAsModule`:
```go
p.compiler.SetHeapAlloc(p.heapAlloc)  // Happens in NewPaserati
chunk, compileAndTypeErrs := p.compiler.Compile(program)
// Compiler records new globals in heapAlloc during compilation
p.vmInstance.SyncGlobalNames(p.compiler.GetHeapAlloc().GetNameToIndexMap())
```

In test262 runner:
```go
// Compiler already has heapAlloc from NewPaserati
chunk, compileErrs := paserati.CompileProgram(prog)
// Compiler records new globals in heapAlloc during compilation
paserati.SyncGlobalNamesFromCompiler()  // Should work the same!
```

**This should be identical!**

---

### Variable Scoping in Concatenated Code

**Hypothesis 4: Globals Declared in Harness Not Visible**

When propertyHelper.js declares:
```javascript
var __join = Function.prototype.call.bind(Array.prototype.join);
```

This creates a global variable `__join`. In the concatenated source, this is at the TOP of the file (after sta.js, assert.js).

**Question**: When we parse the entire concatenated file as one program, does the compiler recognize these as globals?

**In module mode**, `var` declarations at top level are scoped to the module, NOT added to global scope. They're local to the module.

**In script mode**, `var` declarations at top level become global variables.

**CRITICAL DISCOVERY**: The test262 runner compiles the concatenated code in **module mode** by default (because `EnableModuleMode` was called during initialization), but the harness files expect **script mode** semantics!

---

## Hypothesis 5: Module vs Script Mode Mismatch (MOST LIKELY)

### The Problem

**Test262 harness files are written for script mode**, where:
- Top-level `var` declarations become global properties
- Top-level `function` declarations become global properties
- Code at top level runs in global scope

**Paserati test262 runner uses module mode**, where:
- Top-level `var` declarations are module-scoped (local)
- Top-level `function` declarations are module-scoped (local)
- Each module has its own scope

### The Symptom

When propertyHelper.js runs:
```javascript
var __join = Function.prototype.call.bind(Array.prototype.join);
```

At the point this line executes:
1. `Function` is looked up → **works** (it's a builtin global)
2. `Function.prototype` is accessed → **works**
3. `Function.prototype.call` is accessed → **works**
4. `Function.prototype.call.bind` is accessed → **works**
5. `Array` is looked up → **THIS MAY FAIL IN MODULE MODE**

**Why?** In module mode, the lookup might be different. OR the issue is that `Array.prototype` evaluates to `undefined` because of how builtins are accessed in module context.

---

## Hypothesis 6: Compiler Not Running Checker (TYPE CHECKING OFF)

In test262 runner:
```go
paserati.SetIgnoreTypeErrors(true)
```

Then in `CompileProgram`:
```go
p.compiler.SetIgnoreTypeErrors(p.ignoreTypeErrors)
return p.compiler.Compile(program)
```

**Question**: Does `SetIgnoreTypeErrors(true)` skip type checking entirely, which might mean the checker doesn't analyze the code and doesn't see the global references?

**Answer**: Looking at compiler code, `SetIgnoreTypeErrors` tells the compiler to continue compilation even if checker returns errors. The checker still runs, it just doesn't block compilation.

---

## Hypothesis 7: Builtin Globals Not in Compiler's Scope

**Most Likely Root Cause:**

When `initializeBuiltins` runs, it:
1. Creates runtime values in VM (Array constructor, etc.)
2. Stores them in VM heap with indices
3. Creates name-to-index mappings in VM heap

**BUT**: Does it tell the **compiler** about these builtins?

Looking at `initializeBuiltins` in driver.go (around line 940-1040):
```go
// Initialize all builtins runtime values ONCE
for _, init := range initializers {
    currentInitializer = init.Name()
    if err := init.InitRuntime(runtimeCtx); err != nil {
        return fmt.Errorf("failed to initialize %s runtime: %v", init.Name(), err)
    }
}

// Get builtin names and preallocate indices in the heap allocator
// ... code that sets up name->index in heapAlloc ...

// Initialize the heap with builtin globals
if err := vmInstance.SetBuiltinGlobals(standardGlobalVariables, heapAlloc.GetNameToIndexMap()); err != nil {
    return fmt.Errorf("failed to set builtin globals: %v", err)
}

// Set the heap allocator in compiler AFTER we've preallocated builtin indices
// This ensures the compiler knows about all builtins and can resolve references
comp.SetHeapAlloc(heapAlloc)
```

**The flow is**:
1. Initialize builtins → creates values
2. Set up heapAlloc with name→index mappings for builtins
3. Set builtin globals in VM
4. **Give compiler the heapAlloc** so it can resolve global references

**This should work!** The compiler gets the heapAlloc which knows about all builtins.

---

## Hypothesis 8: Checker Not Initialized with Builtin Types

When we create the checker:
```go
typeChecker := checker.NewCheckerWithInitializers(customInitializers)
```

The initializers run `InitTypes` which defines types for builtins like Array, Object, etc.

**BUT**: In module mode, does the checker properly expose these as global types?

Looking at checker initialization... the checker has a type environment that should include builtin types.

---

## Testable Predictions

### Test 1: Check if Array is accessible at all
```bash
# Create minimal test file
echo 'console.log(typeof Array);' > /tmp/test_array.js

# Run with test262 runner
./paserati-test262 -path . -subpath /tmp -pattern "test_array.js"

# Expected: Should print "function"
# If fails: Array is not accessible as a global
```

### Test 2: Check if Array.prototype exists
```bash
echo 'console.log(typeof Array.prototype);' > /tmp/test_array_proto.js
./paserati-test262 -path . -subpath /tmp -pattern "test_array_proto.js"

# Expected: Should print "object"
# If fails: Array constructor doesn't have prototype property
```

### Test 3: Check if propertyHelper __join binding works in isolation
```bash
echo 'var x = Function.prototype.call.bind(Array.prototype.join); console.log(typeof x);' > /tmp/test_join.js
./paserati-test262 -path . -subpath /tmp -pattern "test_join.js"

# Expected: Should print "function"
# If fails: The binding itself is broken
```

### Test 4: Check module vs script mode
```bash
# Test script mode global access
echo 'var testGlobal = 42; function testFunc() { return testGlobal; } console.log(testFunc());' > /tmp/test_globals.js
./paserati-test262 -path . -subpath /tmp -pattern "test_globals.js"

# Expected: Should print "42"
# If fails: Globals don't work as expected
```

### Test 5: Verify SyncGlobalNames is actually called
Add debug logging:
```go
// In pkg/driver/driver.go SyncGlobalNamesFromCompiler():
func (p *Paserati) SyncGlobalNamesFromCompiler() {
	nameMap := p.compiler.GetHeapAlloc().GetNameToIndexMap()
	fmt.Printf("DEBUG: SyncGlobalNames called with %d names\n", len(nameMap))
	for name, idx := range nameMap {
		fmt.Printf("DEBUG:   %s → index %d\n", name, idx)
	}
	p.vmInstance.SyncGlobalNames(nameMap)
}
```

### Test 6: Check what's actually in the heap
Add debug logging to VM:
```go
// In pkg/vm/vm.go GetGlobal():
func (vm *VM) GetGlobal(name string) (Value, bool) {
	if vm != nil && vm.heap != nil {
		if nm, ok := any(vm.heap).(interface{ GetNameToIndex() map[string]int }); ok {
			nameMap := nm.GetNameToIndex()
			fmt.Printf("DEBUG: GetGlobal(%s), heap has %d names\n", name, len(nameMap))
			if _, exists := nameMap[name]; !exists {
				fmt.Printf("DEBUG:   '%s' NOT FOUND in heap name map!\n", name)
			}
			if idx, exists := nameMap[name]; exists {
				val, ok := vm.heap.Get(idx)
				fmt.Printf("DEBUG:   Found '%s' at index %d, type=%s\n", name, idx, val.Type())
				return val, ok
			}
		}
	}
	return Undefined, false
}
```

---

## Most Likely Issues (Ranked)

1. **Module Mode Scoping Issue** (90% confidence)
   - Harness files expect script mode where `Array` is globally accessible
   - Module mode might be scoping things differently
   - **Fix**: Disable module mode for test262 runner, or ensure builtins are accessible in module scope

2. **Compiler HeapAlloc Not Shared** (60% confidence)
   - Each compilation might be creating a new heapAlloc instead of reusing the initialized one
   - **Fix**: Verify compiler.heapAlloc is the same instance as the one initialized with builtins

3. **Checker Not Propagating Types** (40% confidence)
   - With `SetIgnoreTypeErrors(true)`, maybe checker doesn't fully analyze code
   - **Fix**: Ensure checker runs even when errors are ignored

4. **SyncGlobalNames Called Too Late** (30% confidence)
   - Maybe we need to call it BEFORE InterpretChunk?
   - **Fix**: Move SyncGlobalNames earlier

---

## Next Steps

1. **Add debug logging** to SyncGlobalNamesFromCompiler and VM.GetGlobal
2. **Run the failing test** with logging enabled
3. **Check the output** to see:
   - Is SyncGlobalNames being called?
   - What names are in the heap?
   - Is "Array" in the name map?
   - What happens when we try to GetGlobal("Array")?
4. **Based on findings**, implement the appropriate fix


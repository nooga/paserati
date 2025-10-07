# Function Constructor Bug - Root Cause Found

## The Bug

When the Function constructor is invoked (e.g., `new Function("return Array")`), it **corrupts the heap's name-to-index mappings**, causing all previously accessible globals (like `Array`, `Object`, etc.) to become inaccessible.

## Root Cause

**File**: `pkg/builtins/function_init.go` lines 322-326

```go
// Compile and evaluate the function expression
result, errs := d.RunString(source)
```

The Function constructor calls `RunString()` on the same Paserati instance that's executing the current code. This triggers the following sequence:

1. `RunString()` → `runAsModule()`
2. `runAsModule()` compiles the dynamic code
3. **`runAsModule()` calls `SyncGlobalNames()`** which updates the VM heap's nameToIndex map
4. The heap's nameToIndex map is **OVERWRITTEN** with ONLY the names from the dynamically compiled code
5. All previously known globals (Array, Object, etc.) are now missing from the map

## Evidence

From our debugging:
- Array IS in the name map when `SyncGlobalNames` is called initially
- Tests that DON'T use Function constructor work fine
- Tests that use Function constructor (even indirectly) lose access to Array

## The Problem with Current Implementation

```go
// In runAsModule (pkg/driver/driver.go ~line 723):
// Sync global names from compiler to VM heap so globalThis property access works
p.vmInstance.SyncGlobalNames(p.compiler.GetHeapAlloc().GetNameToIndexMap())
```

`SyncGlobalNames` calls `heap.UpdateNameToIndex()` which:
```go
// In pkg/vm/heap.go:
func (h *Heap) UpdateNameToIndex(newMappings map[string]int) {
	if h.nameToIndex == nil {
		h.nameToIndex = make(map[string]int, len(newMappings))
	}
	for name, idx := range newMappings {
		h.nameToIndex[name] = idx  // This OVERWRITES existing mappings!
	}
}
```

When Function constructor's `RunString` compiles `return (function() { return 42 });`, the compiler's heapAlloc only knows about the names in THIS code. When we sync, we overwrite the heap's map with ONLY these names, losing Array, Object, etc.

## Why Test262 Harness Tests Fail

1. Test loads: sta.js + assert.js + propertyHelper.js + test code
2. propertyHelper.js line 29: `var __join = Function.prototype.call.bind(Array.prototype.join);`
3. This line executes fine, Array.prototype.join exists
4. Later, some test might use wellKnownIntrinsicObjects.js which calls `new Function(...)`
5. OR, more subtly: the test itself might trigger Function constructor indirectly
6. Function constructor corrupts the name mappings
7. Later code trying to access Array.prototype.join fails with "Cannot read property 'join' of undefined"

## Solutions

### Option 1: Don't use RunString in Function constructor

Create a separate compilation context that doesn't share state:
- Create a NEW temporary Paserati instance
- Compile the function in that context
- Extract the compiled function value
- Return it to the original context

**Problem**: Functions compiled in one context might not work in another (closures, etc.)

### Option 2: Save and restore heap state

Before calling RunString, save the current nameToIndex map, then restore it after:
```go
// Save current state
originalNameMap := make(map[string]int)
for k, v := range p.compiler.GetHeapAlloc().GetNameToIndexMap() {
	originalNameMap[k] = v
}

// Run the dynamic code
result, errs := d.RunString(source)

// Restore original state
p.vmInstance.SyncGlobalNames(originalNameMap)
```

**Problem**: This is hacky and might not handle all edge cases.

### Option 3: Fix UpdateNameToIndex to merge, not replace (PREFERRED)

The real issue is that `UpdateNameToIndex` should MERGE new mappings with existing ones, not replace them. But wait - it already does merge:

```go
for name, idx := range newMappings {
	h.nameToIndex[name] = idx  // This adds OR updates, doesn't delete
}
```

So why are globals disappearing?

**AH!** The problem is that the COMPILER's heapAlloc is being RESET or not properly maintaining state between compilations!

### Option 4: Don't call SyncGlobalNames in nested RunString

The Function constructor's RunString call should NOT sync global names back to the heap. We need a way to compile and execute code without affecting the parent session's name mappings.

Add a `RunStringWithoutSync()` method that skips the SyncGlobalNames step.

## Recommended Fix

**Implement Option 4**: Add a flag to control whether `runAsModule` syncs global names:

```go
// Add parameter to runAsModule
func (p *Paserati) runAsModule(sourceCode string, program *parser.Program, moduleName string, syncNames bool) (vm.Value, []errors.PaseratiError) {
	// ... existing code ...

	// Only sync if requested
	if syncNames {
		p.vmInstance.SyncGlobalNames(p.compiler.GetHeapAlloc().GetNameToIndexMap())
	}

	// ... rest of code ...
}

// Add new method for Function constructor
func (p *Paserati) RunStringWithoutSync(sourceCode string) (vm.Value, []errors.PaseratiError) {
	// ... parse code ...
	return p.runAsModule(sourceCode, program, "__dynamic__", false)
}
```

Then in Function constructor, use `RunStringWithoutSync` instead of `RunString`.

## Test to Verify Fix

```javascript
console.log("1. Array type:", typeof Array);
const fn = new Function("return 42");
console.log("2. After Function constructor, Array type:", typeof Array);
console.log("3. Array.prototype.join type:", typeof Array.prototype.join);
```

Expected output:
```
1. Array type: function
2. After Function constructor, Array type: function
3. Array.prototype.join type: function
```

Current behavior: Step 2 or 3 would show `undefined`.

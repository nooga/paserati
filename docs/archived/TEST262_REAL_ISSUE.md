# Test262 Real Issue: Non-Deterministic Initialization

## Summary

The test262 harness tests show **flaky behavior** - pass rates vary between 84-90 out of 116 tests between runs. Some tests fail with "Cannot read property 'join' of undefined" inconsistently.

## What We Know

1. ✅ `Array` IS in the compiler's heapAlloc nameToIndex map
2. ✅ `SyncGlobalNames` is called and merges names into VM heap
3. ✅ paserati binary (`./paserati`) works perfectly - Array.prototype.join accessible
4. ✅ Function constructor was a red herring (fixed but not the root cause)
5. ❌ test262 runner shows flaky behavior - same test passes or fails randomly
6. ❌ Pass rate varies: 84, 86, 89, 90 across different runs

## Root Cause: Non-Deterministic Map Iteration

**In Go, map iteration order is randomized**. We're iterating over maps during initialization:

### Smoking Gun Code

**pkg/driver/driver.go** around line 960-1010 in `initializeBuiltinsWithCustom`:

```go
// Track globals defined by each initializer during the SINGLE initialization pass
globalsPerInitializer := make(map[string][]string)
currentInitializer := ""

runtimeCtx := &builtins.RuntimeContext{
	VM:     vmInstance,
	Driver: paserati,
	DefineGlobal: func(name string, value vm.Value) error {
		globalVariables[name] = value  // ← MAP INSERTION
		// ...
	},
}

// Initialize all builtins runtime values ONCE
for _, init := range initializers {  // ← ORDER MATTERS
	currentInitializer = init.Name()
	if err := init.InitRuntime(runtimeCtx); err != nil {
		return fmt.Errorf("failed to initialize %s runtime: %v", init.Name(), err)
	}
}

// ... later ...

// Set up heapAlloc with name->index mappings
for name, value := range standardGlobalVariables {  // ← RANDOM ORDER!
	if existingIdx, exists := heapAlloc.GetIndex(name); exists {
		// Use existing index
	} else {
		// Allocate new index - BUT ORDER IS RANDOM!
		idx := heapAlloc.GetOrAssignIndex(name)
	}
}
```

The problem: When we iterate over `standardGlobalVariables` (a map), the order is random. This means **Array might get index 0 in one run and index 5 in another run**.

If there's any code that relies on **stable indices** or **initialization order**, it will break randomly.

## Why This Manifests as "join" Errors

When propertyHelper.js runs:
```javascript
var __join = Function.prototype.call.bind(Array.prototype.join);
```

This line evaluates `Array.prototype.join` DURING the harness file loading. If `Array` hasn't been properly stored in the heap yet (due to initialization order), it might be undefined.

## The Real Fix

**Make initialization order deterministic**:

1. Sort map keys before iterating
2. Use consistent ordering for builtin initialization
3. Ensure heapAlloc indices are stable

### Specific Changes Needed

**In pkg/driver/driver.go**, around line 1000:

```go
// Get builtin names in SORTED order for deterministic indices
var standardNames []string
for name := range standardGlobalVariables {
	if standardInitSet[initializer name for this global] {
		standardNames = append(standardNames, name)
	}
}
sort.Strings(standardNames)  // ← CRITICAL: Sort for determinism

// Preallocate indices in sorted order
for _, name := range standardNames {  // ← Now deterministic!
	if existingIdx, exists := heapAlloc.GetIndex(name); !exists {
		heapAlloc.GetOrAssignIndex(name)
	}
}
```

Actually, looking at the code more carefully, there's already a `PreallocateBuiltins` method that sorts! Let me check if it's being used...

Looking at heap_alloc.go line 78-90:
```go
// PreallocateBuiltins sets up indices for builtin globals in alphabetical order
// This ensures consistent ordering across all compilers
func (ha *HeapAlloc) PreallocateBuiltins(builtinNames []string) {
	// Sort to ensure consistent ordering
	sortedNames := make([]string, len(builtinNames))
	copy(sortedNames, builtinNames)
	sort.Strings(sortedNames)

	// Assign indices starting from 0
	for i, name := range sortedNames {
		ha.SetIndex(name, i)
	}
}
```

So there IS sorting! But maybe it's not being called correctly, or the sorted order is being violated later?

## Investigation Needed

1. Verify `PreallocateBuiltins` is actually called
2. Check if sorted order is maintained through entire initialization
3. Look for any map iteration that could randomize order
4. Add logging to track actual indices assigned to builtins

## Quick Test

Add this logging to see if indices are stable:

```go
// In initializeBuiltinsWithCustom after heapAlloc setup
names := heapAlloc.GetAllNames()  // Returns sorted
fmt.Printf("[INIT] Builtin indices:\n")
for _, name := range names {
	if idx, exists := heapAlloc.GetIndex(name); exists {
		fmt.Printf("  %s → %d\n", name, idx)
	}
}
```

Run the same test multiple times and see if indices change.

## Expected Outcome

After fixing non-determinism:
- ✅ Tests should have consistent pass/fail (not flaky)
- ✅ Either all property helper tests pass, or they all fail for the SAME reason
- ✅ Can debug actual issues instead of chasing phantoms

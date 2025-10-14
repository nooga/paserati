# Known Issues in Paserati

## Proxy Handler Closures Bug

**Status**: FIXED ✅

**Description**: Proxy handler trap methods that close over external variables were failing with "'trap' on proxy: trap is not a function" error.

**Root Cause**: The trap validation code was using `IsFunction()` which only checks for `TypeFunction`, but closures have type `TypeClosure`. The fix was to use `IsCallable()` instead, which checks for all callable types including closures.

**Fix Applied**: Replaced all `IsFunction()` calls with `IsCallable()` in trap validation code across:
- `pkg/vm/op_getprop.go` (get trap)
- `pkg/vm/op_setprop.go` (set trap)
- `pkg/vm/vm.go` (has, get, construct, deleteProperty traps)
- `pkg/vm/call.go` (apply trap)

**Commit**: All 13 Proxy traps now correctly handle closure methods with upvalues.

## Proxy Trap Invocation Breaks Class Inheritance

**Status**: FIXED ✅

**Description**: After calling any Proxy trap method, subsequent class declarations with `extends` would cause the program to hang during super() calls.

**Root Cause**: When Proxy traps were called via `vm.Call()`, a sentinel frame was pushed onto the frame stack with `isSentinelFrame = true`. When the nested `vm.run()` returned, it decremented `frameCount` but left the sentinel frame struct in the `vm.frames` array with its flag still set to `true`. Later, when creating new call frames for class constructors, the code reused these frame structs without explicitly clearing `isSentinelFrame`, causing constructor frames to be incorrectly treated as sentinel frames. This made the VM return prematurely from the OpReturnUndefined handler, breaking the super() call flow.

**Fix Applied**: Added explicit `isSentinelFrame = false` initialization when creating new call frames in:
- `pkg/vm/call.go` (line 219) - prepareCall function
- `pkg/vm/vm.go` (lines 3503, 3645) - OpNew cases for closures and functions
- `pkg/vm/op_spreadnew.go` (lines 71, 133) - OpSpreadNew cases

Additionally added `isDirectCall = false` initialization to ensure all frame state is properly reset when reusing frame structs from the fixed-size `vm.frames` array.

**Regression Test**: Added `tests/scripts/proxy_trap_inheritance_test.ts` to prevent this bug from reoccurring.

**Impact**: The comprehensive `demo.ts` and `demo_reactive.ts` now work correctly end-to-end, showcasing Proxy/Reflect API alongside class inheritance without hanging.

## Map and Set Boolean Conversion Bug

**Status**: FIXED ✅

**Description**: Map and Set objects were incorrectly treated as falsy in boolean contexts, causing reactive systems using Maps/Sets for dependency tracking to fail silently.

**Root Cause**: The `IsFalsey()` function in `pkg/vm/value.go` listed specific object types that should be truthy (line 1233), but Map and Set types were missing from the list. This caused boolean conversion (`!map`, `!set`, `if (map)`) to incorrectly treat Map and Set instances as falsy.

**Fix Applied**: Added all missing object types to the truthy case in `IsFalsey()`:
- `TypeMap`, `TypeSet` (the main culprits)
- `TypeDictObject`, `TypeBoundFunction`, `TypeNativeFunctionWithProps`
- `TypeAsyncNativeFunction`, `TypeGenerator`, `TypeAsyncGenerator`
- `TypeArrayBuffer`, `TypeTypedArray`

All object types in JavaScript should be truthy, so this ensures comprehensive coverage.

**Additional Fix**: Implemented `Set.prototype.forEach()` method which was missing:
- Added runtime implementation in `pkg/builtins/set_init.go`
- Added type definition for forEach method signature
- Follows ECMAScript spec: callback receives `(value, value, set)` - value is passed twice for consistency with Map

**Regression Test**: Added `tests/scripts/map_set_truthiness.ts` to prevent this bug from reoccurring.

**Impact**: The reactive demo (`demo_reactive.ts`) now works correctly, showcasing:
- Automatic dependency tracking with Proxy/Reflect
- Computed properties with caching
- Derived values with dependency chains
- Multiple reactive objects with proper effect triggering

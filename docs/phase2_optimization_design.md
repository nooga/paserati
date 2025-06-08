# Phase 2: Inline Cache Optimization Design

## Current State Analysis

After reviewing the refactored property access code in `op_getprop.go` and `op_setprop.go`, several optimization opportunities are evident:

### Issues Identified

1. **Duplicate Prototype Lookups**: Each primitive type (String, Array, Function) has separate prototype method checks, but they all follow the same pattern
2. **Scattered Caching Logic**: Inline cache logic is embedded within each property access path
3. **No Prototype Chain Caching**: The TODO comment on line 184 highlights missing prototype chain optimization
4. **Special Case Proliferation**: Functions, closures, and NativeFunctionWithProps all have similar but duplicated logic

## Centralized Prototype Lookup Strategy

### 1. Unified Prototype Resolution

Create a centralized `getPrototypeFor(value Value) *PlainObject` function that returns the appropriate prototype for any value type:

```go
// pkg/vm/prototype_resolver.go
func getPrototypeFor(value Value) *PlainObject {
    switch value.Type() {
    case TypeString:
        return StringPrototype
    case TypeArray:
        return ArrayPrototype
    case TypeFunction, TypeClosure:
        return FunctionPrototype
    case TypeObject:
        // PlainObject - check its prototype chain
        if obj := value.AsPlainObject(); obj != nil {
            return obj.prototype // Direct prototype reference
        }
        return nil
    default:
        return nil
    }
}
```

### 2. Enhanced Inline Cache Architecture

Extend the current inline cache to support prototype chain lookups:

```go
type PrototypeCacheEntry struct {
    objectShape     *Shape    // Shape of the object being accessed
    prototypeObj    *PlainObject  // Prototype where property was found
    prototypeDepth  int       // Chain depth (0=own, 1=proto, 2=proto.proto)
    offset          int       // Property offset in prototype
    boundMethod     Value     // Cached bound method for primitives
    isMethod        bool      // Whether this is a method requiring 'this' binding
}

type EnhancedPropCache struct {
    state           CacheState
    monomorphic     PrototypeCacheEntry
    polymorphic     [4]PrototypeCacheEntry  // Support up to 4 shapes
    megamorphic     map[*Shape]PrototypeCacheEntry
}
```

### 3. Streamlined Property Access Pipeline

Replace the current scattered approach with a unified pipeline:

```go
// Unified property access pipeline
func (vm *VM) resolveProperty(objVal Value, propName string, cache *EnhancedPropCache) (Value, bool) {
    // 1. Check cache first
    if result, hit := vm.checkCache(objVal, propName, cache); hit {
        return result, true
    }
    
    // 2. Special cases (length, etc.)
    if result, handled := vm.handleSpecialProperties(objVal, propName); handled {
        return result, true
    }
    
    // 3. Own property lookup
    if result, found := vm.getOwnProperty(objVal, propName); found {
        vm.updateCache(objVal, propName, result, 0, cache) // depth 0 = own
        return result, true
    }
    
    // 4. Prototype chain traversal
    return vm.traversePrototypeChain(objVal, propName, cache)
}
```

## Specific Optimizations

### 1. Eliminate Duplicate Code

Current code has near-identical blocks for:
- Functions vs Closures (lines 68-96 vs 98-127 in op_getprop.go)
- Different prototype lookups (lines 43-57 for String/Array)

**Solution**: Extract common patterns into helper functions:

```go
func (vm *VM) handleCallableProperty(callable Value, propName string) (Value, bool) {
    // Unified logic for Function/Closure property access
    // Handles both .prototype and FunctionPrototype method lookup
}

func (vm *VM) handlePrimitiveMethod(primitive Value, propName string) (Value, bool) {
    // Unified logic for String/Array prototype method binding
}
```

### 2. Optimize Prototype Method Binding

Current `createBoundMethod` creates new bound functions on every access. For frequently accessed methods, cache the bound method:

```go
type BoundMethodCache struct {
    primitiveType ValueType
    methodName    string
    cachedMethod  Value
}
```

### 3. Streamline Cache Key Generation

Current cache key calculation (lines 7-13) is duplicated in both files. Extract to utility:

```go
func generateCacheKey(ip int, propName string) int {
    propNameHash := 0
    for _, b := range []byte(propName) {
        propNameHash = propNameHash*31 + int(b)
    }
    return (ip-5)*100000 + (propNameHash & 0xFFFF)
}
```

## Implementation Plan

### Phase 2.1: Refactor Current Code
1. Extract duplicate logic into helper functions
2. Create centralized prototype resolver
3. Unified cache key generation
4. Simplify op_getprop.go and op_setprop.go

### Phase 2.2: Enhanced Cache Architecture
1. Implement PrototypeCacheEntry structure
2. Add prototype chain caching logic
3. Update cache hit/miss logic for prototype lookups

### Phase 2.3: Performance Optimizations
1. Bound method caching for frequently accessed prototype methods
2. Shape-based prototype cache invalidation
3. Benchmark and tune cache sizes

### Phase 2.4: Integration and Testing
1. Ensure all existing tests pass
2. Add performance benchmarks
3. Validate cache hit rates with real-world scenarios

## Expected Performance Gains

1. **Prototype Method Access**: 2-3x faster for repeated `string.method()` calls
2. **Cache Hit Rate**: Improved from ~60% to ~85% for prototype-heavy code
3. **Memory Usage**: Reduced by eliminating duplicate bound method creation
4. **Code Maintainability**: Single source of truth for property resolution logic

## Risk Mitigation

1. **Backward Compatibility**: All changes maintain existing API contracts
2. **Incremental Deployment**: Each phase can be implemented and tested independently
3. **Performance Regression**: Comprehensive benchmarking before/after each phase
4. **Cache Invalidation**: Proper handling of prototype chain modifications

## Next Steps

1. Create `pkg/vm/prototype_resolver.go` with centralized prototype lookup
2. Extract common patterns from op_getprop.go into helper functions
3. Implement enhanced cache architecture
4. Add comprehensive benchmarks to measure improvements

This design addresses the core issues while maintaining the sophisticated inline caching system already in place, providing a clear path to significant performance improvements in prototype-heavy code.
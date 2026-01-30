# Builtin Implementation Guide

This document describes the canonical patterns for implementing ECMAScript-compliant builtins in Paserati. Following these patterns is critical for Test262 compliance.

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Constructor and Prototype Setup](#constructor-and-prototype-setup)
3. [Error Handling and Propagation](#error-handling-and-propagation)
4. [Callback Methods (map, reduce, forEach, etc.)](#callback-methods)
5. [Property Descriptors](#property-descriptors)
6. [Type Checking](#type-checking)
7. [Common Anti-Patterns](#common-anti-patterns)
8. [Checklist for New Builtins](#checklist-for-new-builtins)

---

## Architecture Overview

Each builtin is implemented as a struct implementing `BuiltinInitializer`:

```go
type BuiltinInitializer interface {
    Name() string           // e.g., "Array", "Math"
    Priority() int          // Initialization order (lower = earlier)
    InitTypes(ctx *TypeContext) error     // Type definitions for checker
    InitRuntime(ctx *RuntimeContext) error // Runtime values for VM
}
```

**Priority Order** (see `initializer.go`):
- Object: 0 (root prototype)
- Function: 1 (needed for all functions)
- Array: 3
- Core types: 10-20
- Other: 100+

---

## Constructor and Prototype Setup

### The Prototype Chain

```
Object.prototype (root, [[Prototype]] = null)
    ↑
Function.prototype (callable, [[Prototype]] = Object.prototype)
    ↑
Array.prototype, String.prototype, etc.
```

### Canonical Constructor Setup

```go
func (x *XInitializer) InitRuntime(ctx *RuntimeContext) error {
    vmInstance := ctx.VM

    // 1. Get Object.prototype for inheritance
    objectProto := vmInstance.ObjectPrototype

    // 2. Create X.prototype inheriting from Object.prototype
    xProto := vm.NewObject(objectProto).AsPlainObject()

    // 3. Add prototype methods (non-enumerable)
    xProto.SetOwnNonEnumerable("methodName", vm.NewNativeFunction(arity, variadic, "methodName", func(args []vm.Value) (vm.Value, error) {
        // Implementation
    }))

    // 4. Create constructor with NewConstructorWithProps
    xConstructor := vm.NewConstructorWithProps(arity, variadic, "X", func(args []vm.Value) (vm.Value, error) {
        // Handle both new X() and X() calls
        if vmInstance.IsConstructorCall() {
            // Return new instance
        }
        // Return converted value
    })

    // 5. Link prototype ↔ constructor (BOTH directions!)
    xConstructor.AsNativeFunctionWithProps().Properties.SetOwnNonEnumerable("prototype", vm.NewValueFromPlainObject(xProto))
    xProto.SetOwnNonEnumerable("constructor", xConstructor)

    // 6. Store prototype in VM if needed
    vmInstance.XPrototype = vm.NewValueFromPlainObject(xProto)

    // 7. Register as global
    return ctx.DefineGlobal("X", xConstructor)
}
```

### Key Points

1. **Object.prototype** is created with `vm.NewObject(vm.Null)` - it's the root
2. **Other prototypes** use `vm.NewObject(objectProto)` to inherit
3. **NewConstructorWithProps** creates a constructor with `.prototype` property support
4. **Bidirectional linking**: Constructor → prototype AND prototype → constructor
5. **Use SetOwnNonEnumerable** for methods (they should not appear in for-in loops)

---

## Error Handling and Propagation

### CRITICAL: Use vmInstance.NewTypeError, NOT fmt.Errorf

❌ **WRONG** - Creates a Go error, not catchable in JavaScript:
```go
return vm.Undefined, fmt.Errorf("TypeError: invalid argument")
```

✅ **CORRECT** - Creates a proper JavaScript TypeError:
```go
return vm.Undefined, vmInstance.NewTypeError("invalid argument")
```

### Available Error Constructors

```go
vmInstance.NewTypeError(message string) error      // Most common
vmInstance.NewRangeError(message string) error     // Out of bounds, invalid length
vmInstance.NewReferenceError(message string) error // Undefined variable access
vmInstance.NewExceptionError(value Value) error    // Wrap any Value as exception
```

### Error Propagation from Callbacks

When calling user-provided callbacks, **always check and propagate errors**:

```go
result, err := vmInstance.Call(callback, thisArg, args)
if err != nil {
    return vm.Undefined, err  // Propagate immediately
}
```

---

## Callback Methods

Methods like `map`, `reduce`, `forEach` must follow the ECMAScript specification exactly.

### Canonical Pattern for Iteration Methods

```go
arrayProto.SetOwnNonEnumerable("map", vm.NewNativeFunction(1, false, "map", func(args []vm.Value) (vm.Value, error) {
    // 1. Let O be ? ToObject(this value).
    thisVal := vmInstance.GetThis()
    if thisVal.Type() == vm.TypeUndefined || thisVal.Type() == vm.TypeNull {
        return vm.Undefined, vmInstance.NewTypeError("Array.prototype.map called on null or undefined")
    }

    // 2. Let len be ? LengthOfArrayLike(O). - MUST access length BEFORE checking callback
    var length int
    if arr := thisVal.AsArray(); arr != nil {
        length = arr.Length()
    } else if po := thisVal.AsPlainObject(); po != nil {
        if lv, ok := po.Get("length"); ok {
            length = int(lv.ToFloat())
            if length < 0 {
                length = 0
            }
        }
    }

    // 3. If IsCallable(callbackfn) is false, throw a TypeError exception.
    var callback vm.Value
    if len(args) >= 1 {
        callback = args[0]
    } else {
        callback = vm.Undefined
    }
    if !callback.IsCallable() {
        callbackStr := "undefined"
        if callback.Type() != vm.TypeUndefined {
            callbackStr = callback.ToString()
        }
        return vm.Undefined, vmInstance.NewTypeError(fmt.Sprintf("%s is not a function", callbackStr))
    }

    // 4. Get thisArg (second argument)
    var thisArg vm.Value
    if len(args) >= 2 {
        thisArg = args[1]
    } else {
        thisArg = vm.Undefined
    }

    // 5. Create result array
    result := vm.NewArray()
    resultArr := result.AsArray()
    resultArr.SetLength(length)

    // 6. Iterate and call callback for each existing element
    if arr := thisVal.AsArray(); arr != nil {
        for i := 0; i < length; i++ {
            // Only call callback for indices that actually exist (sparse array support)
            if arr.HasIndex(i) {
                element := arr.Get(i)
                mappedValue, err := vmInstance.Call(callback, thisArg, []vm.Value{element, vm.NumberValue(float64(i)), thisVal})
                if err != nil {
                    return vm.Undefined, err  // CRITICAL: Propagate callback errors
                }
                resultArr.Set(i, mappedValue)
            }
        }
        return result, nil
    }

    // Handle array-like objects too
    // ...

    return result, nil
}))
```

### Key Requirements

1. **Order matters**: Access `length` BEFORE checking if callback is callable
2. **Sparse arrays**: Use `HasIndex(i)` to skip holes
3. **thisArg**: Always respect the second argument as `this` for callback
4. **Error propagation**: Check `err != nil` after every `vmInstance.Call()`
5. **Array-like objects**: Support objects with `length` property

---

## Property Descriptors

### Standard Property Attributes

| Property Type | writable | enumerable | configurable |
|---------------|----------|------------|--------------|
| Prototype methods | true | **false** | true |
| Constants (Math.PI) | **false** | **false** | **false** |
| Array length | true | **false** | **false** |
| Constructor.prototype | **false** | **false** | **false** |

### Setting Attributes Explicitly

```go
// After setting a method, ensure proper attributes
if v, ok := proto.GetOwn("methodName"); ok {
    w, e, c := true, false, true  // writable, not enumerable, configurable
    proto.DefineOwnProperty("methodName", v, &w, &e, &c)
}

// For constants
f := false
mathObj.DefineOwnProperty("PI", vm.NumberValue(math.Pi), &f, &f, &f)

// For @@toStringTag
falseVal := false
trueVal := true
obj.DefineOwnPropertyByKey(
    vm.NewSymbolKey(vmInstance.SymbolToStringTag),
    vm.NewString("Math"),
    &falseVal, // writable: false
    &falseVal, // enumerable: false
    &trueVal,  // configurable: true (per spec)
)
```

---

## Type Checking

### Validate `this` Before Operations

```go
func(args []vm.Value) (vm.Value, error) {
    thisVal := vmInstance.GetThis()

    // For methods that require specific type
    if thisVal.Type() != vm.TypeSet {
        return vm.Undefined, vmInstance.NewTypeError("Method Set.prototype.add called on incompatible receiver")
    }

    // Now safe to use
    setObj := thisVal.AsSet()
    // ...
}
```

### Check Callback Callability

```go
if !callback.IsCallable() {
    return vm.Undefined, vmInstance.NewTypeError("callback is not a function")
}
```

### Type-Specific Errors

Follow ECMAScript error message conventions:
- `"X.prototype.method called on incompatible receiver"`
- `"X.prototype.method called on null or undefined"`
- `"callback is not a function"`
- `"Reduce of empty array with no initial value"`

---

## Common Anti-Patterns

### ❌ Using fmt.Errorf for JavaScript Errors

```go
// WRONG - Not catchable in JavaScript try/catch
return vm.Undefined, fmt.Errorf("Invalid ArrayBuffer length")

// CORRECT
return vm.Undefined, vmInstance.NewRangeError("Invalid ArrayBuffer length")
```

### ❌ Discarding Callback Errors

```go
// WRONG - Error is discarded
setObj.ForEach(func(val vm.Value) {
    _, _ = vmInstance.Call(callback, thisArg, []vm.Value{val, val, thisSet})
})

// CORRECT - Propagate errors
for _, val := range setObj.Values() {
    _, err := vmInstance.Call(callback, thisArg, []vm.Value{val, val, thisSet})
    if err != nil {
        return vm.Undefined, err
    }
}
```

### ❌ Silently Returning Instead of Throwing

```go
// WRONG - Returns undefined instead of throwing
if thisSet.Type() != vm.TypeSet {
    return vm.Undefined, nil
}

// CORRECT - Throws proper TypeError
if thisSet.Type() != vm.TypeSet {
    return vm.Undefined, vmInstance.NewTypeError("Set.prototype.add called on incompatible receiver")
}
```

### ❌ Missing Bidirectional Prototype/Constructor Link

```go
// WRONG - Only one direction
ctorProps.Properties.SetOwnNonEnumerable("prototype", proto)

// CORRECT - Both directions
ctorProps.Properties.SetOwnNonEnumerable("prototype", proto)
proto.SetOwnNonEnumerable("constructor", ctor)
```

### ❌ Using Enumerable Methods

```go
// WRONG - Method shows up in for-in loops
proto.SetOwn("forEach", vm.NewNativeFunction(...))

// CORRECT - Method is non-enumerable
proto.SetOwnNonEnumerable("forEach", vm.NewNativeFunction(...))
```

---

## Checklist for New Builtins

### Structure
- [ ] Implement `BuiltinInitializer` interface
- [ ] Set appropriate priority (check dependencies)
- [ ] Implement both `InitTypes` and `InitRuntime`

### Prototype Setup
- [ ] Create prototype inheriting from `ObjectPrototype`
- [ ] Add all required methods as non-enumerable
- [ ] Create constructor with `NewConstructorWithProps`
- [ ] Link prototype ↔ constructor bidirectionally
- [ ] Set `@@toStringTag` if applicable

### Methods
- [ ] Use `vm.NewNativeFunction(arity, variadic, name, fn)`
- [ ] Validate `this` type at start
- [ ] Use `vmInstance.NewTypeError()` for errors (not `fmt.Errorf`)
- [ ] For callback methods: propagate errors from `vmInstance.Call()`
- [ ] Support `thisArg` parameter where specified
- [ ] Handle sparse arrays with `HasIndex()`

### Property Descriptors
- [ ] Methods: writable, not enumerable, configurable
- [ ] Constants: not writable, not enumerable, not configurable
- [ ] Use `DefineOwnProperty` for explicit control

### Type Checking
- [ ] Check `this` is the correct type
- [ ] Check callback arguments are callable
- [ ] Check null/undefined receivers

### Error Messages
- [ ] Follow ECMAScript conventions
- [ ] Include method name in error messages
- [ ] Use proper error constructor (TypeError, RangeError, etc.)

### Testing
- [ ] Run Test262 suite for the builtin
- [ ] Check error messages match expected patterns
- [ ] Test edge cases (empty arrays, sparse arrays, null this)

---

## Example: Well-Implemented Builtin

See these files for reference implementations:
- `array_init.go` - Array with callback methods (91.9% pass rate)
- `boolean_init.go` - Simple wrapper type (100% pass rate)
- `math_init.go` - Namespace object with static methods (100% pass rate)
- `string_init.go` - String with prototype methods (91.4% pass rate)

For each failing builtin, compare against these patterns and look for:
1. Missing error propagation
2. Wrong error types (fmt.Errorf vs NewTypeError)
3. Missing type validation
4. Incorrect property attributes
5. Missing prototype chain setup

# Builtin System Refactor Design (Simplified)

## Executive Summary

This document outlines a simplified refactor of Paserati's builtin system. The key insight is that we can use the existing object creation and property assignment mechanisms instead of creating special registration methods.

## Core Design Principles

1. **Use Native Mechanisms**: Create objects and set properties using existing VM/type system methods
2. **Direct Assignment**: Prototypes are just objects with properties - create them directly
3. **No Special APIs**: No need for `DefinePrototypeMethod` - just use `SetOwn`
4. **Minimal Interface**: Keep the initializer interface as simple as possible

## Simplified Interface Design

```go
// pkg/builtins/initializer.go

// BuiltinInitializer is implemented by each builtin module
type BuiltinInitializer interface {
    // Name returns the module name (e.g., "Array", "String", "Math")
    Name() string

    // Priority returns initialization order (lower = earlier)
    Priority() int

    // InitTypes creates type definitions for the checker
    InitTypes(ctx *TypeContext) error

    // InitRuntime creates runtime values for the VM
    InitRuntime(ctx *RuntimeContext) error
}

// TypeContext provides everything needed for type initialization
type TypeContext struct {
    // Define a global type (constructor, namespace, etc.)
    DefineGlobal func(name string, typ types.Type) error

    // Get a previously defined type
    GetType func(name string) (types.Type, bool)

    // Store prototype types for primitives (for checker's getBuiltinType)
    SetPrimitivePrototype func(primitiveName string, prototypeType *types.ObjectType)
}

// RuntimeContext provides everything needed for runtime initialization
type RuntimeContext struct {
    // The VM instance
    VM *vm.VM

    // Define a global value
    DefineGlobal func(name string, value vm.Value) error

    // Get built-in prototypes
    ObjectPrototype vm.Value
    FunctionPrototype vm.Value
    // ... others as needed
}
```

## Example Implementation: Object Builtin

```go
// pkg/builtins/object_init.go

type ObjectInitializer struct{}

func (o *ObjectInitializer) Name() string { return "Object" }
func (o *ObjectInitializer) Priority() int { return 0 } // First

func (o *ObjectInitializer) InitTypes(ctx *TypeContext) error {
    // Create Object.prototype type using fluent API
    objectProtoType := types.NewObjectType().
        WithProperty("hasOwnProperty", types.NewSimpleFunction([]types.Type{types.String}, types.Boolean)).
        WithProperty("toString", types.NewSimpleFunction([]types.Type{}, types.String)).
        WithProperty("valueOf", types.NewSimpleFunction([]types.Type{}, types.Any)).
        WithProperty("isPrototypeOf", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean))

    // Create Object constructor type using fluent API
    objectCtorType := types.NewObjectType().
        // Constructor is callable
        WithSimpleCallSignature([]types.Type{}, objectProtoType).
        WithSimpleCallSignature([]types.Type{types.Any}, types.Any).
        // Static methods
        WithProperty("create", types.NewSimpleFunction([]types.Type{types.Any}, types.NewObjectType())).
        WithProperty("keys", types.NewSimpleFunction([]types.Type{types.Any}, &types.ArrayType{ElementType: types.String})).
        WithProperty("prototype", objectProtoType)

    // Define the constructor globally
    if err := ctx.DefineGlobal("Object", objectCtorType); err != nil {
        return err
    }

    // Store the prototype type for primitive "object"
    ctx.SetPrimitivePrototype("object", objectProtoType)

    return nil
}

func (o *ObjectInitializer) InitRuntime(ctx *RuntimeContext) error {
    vm := ctx.VM

    // Create Object.prototype
    objectProto := vm.NewObject(vm.Null).AsPlainObject()

    // Add prototype methods
    objectProto.SetOwn("hasOwnProperty", vm.NewNativeFunction(1, false, "hasOwnProperty", func(args []vm.Value) vm.Value {
        if len(args) < 2 {
            return vm.BooleanValue(false)
        }
        thisValue := args[0]
        propName := args[1].ToString()

        if thisValue.IsObject() {
            if obj := thisValue.AsPlainObject(); obj != nil {
                return vm.BooleanValue(obj.HasOwn(propName))
            }
            if dict := thisValue.AsDictObject(); dict != nil {
                return vm.BooleanValue(dict.HasOwn(propName))
            }
        }
        return vm.BooleanValue(false)
    }))

    objectProto.SetOwn("toString", vm.NewNativeFunction(0, false, "toString", func(args []vm.Value) vm.Value {
        if len(args) < 1 {
            return vm.String("[object Object]")
        }
        // TODO: Implement proper toString
        return vm.String("[object Object]")
    }))

    objectProto.SetOwn("valueOf", vm.NewNativeFunction(0, false, "valueOf", func(args []vm.Value) vm.Value {
        if len(args) < 1 {
            return vm.Undefined
        }
        return args[0] // Return this
    }))

    objectProto.SetOwn("isPrototypeOf", vm.NewNativeFunction(1, false, "isPrototypeOf", func(args []vm.Value) vm.Value {
        // Implementation...
        return vm.BooleanValue(false)
    }))

    // Create Object constructor
    objectCtor := vm.NewNativeFunction(-1, true, "Object", func(args []vm.Value) vm.Value {
        if len(args) == 0 {
            return vm.NewObject(objectProto)
        }
        arg := args[0]
        if arg.IsObject() {
            return arg
        }
        // TODO: Box primitives
        return vm.NewObject(objectProto)
    })

    // Make it a proper constructor with static methods
    if ctorObj := objectCtor.AsNativeFunction(); ctorObj != nil {
        // Convert to object with properties
        ctorWithProps := vm.NewNativeFunctionWithProps(ctorObj.Arity, ctorObj.Variadic, ctorObj.Name, ctorObj.Fn)
        ctorObj = ctorWithProps.AsNativeFunctionWithProps()

        // Add prototype property
        ctorObj.Props.SetOwn("prototype", objectProto)

        // Add static methods
        ctorObj.Props.SetOwn("create", vm.NewNativeFunction(1, false, "create", objectCreateImpl))
        ctorObj.Props.SetOwn("keys", vm.NewNativeFunction(1, false, "keys", objectKeysImpl))

        objectCtor = ctorWithProps
    }

    // Set constructor property on prototype
    objectProto.SetOwn("constructor", objectCtor)

    // Store in VM
    vm.ObjectPrototype = objectProto

    // Define globally
    return ctx.DefineGlobal("Object", objectCtor)
}
```

## Integration Examples

### Checker Integration

```go
// pkg/checker/environment.go

func NewGlobalEnvironment() *Environment {
    env := &Environment{
        store:    make(map[string]binding),
        outer:    nil,
        isGlobal: true,
    }

    // Create primitive prototype registry
    primitivePrototypes := make(map[string]*types.ObjectType)

    // Create type context
    typeCtx := &builtins.TypeContext{
        DefineGlobal: func(name string, typ types.Type) error {
            if !env.Define(name, typ, false) {
                return fmt.Errorf("global %s already defined", name)
            }
            return nil
        },
        GetType: func(name string) (types.Type, bool) {
            typ, _, found := env.Resolve(name)
            return typ, found
        },
        SetPrimitivePrototype: func(primitiveName string, prototypeType *types.ObjectType) {
            primitivePrototypes[primitiveName] = prototypeType
        },
    }

    // Initialize all builtins
    initializers := builtins.GetStandardInitializers()
    for _, init := range initializers {
        if err := init.InitTypes(typeCtx); err != nil {
            panic(fmt.Sprintf("failed to initialize %s types: %v", init.Name(), err))
        }
    }

    // Store primitive prototypes for later use
    env.primitivePrototypes = primitivePrototypes

    return env
}

// Update getBuiltinType to use stored prototypes
func (c *Checker) getBuiltinType(name string) types.Type {
    // Check for global builtins first
    if typ, _, found := c.env.Resolve(name); found {
        return typ
    }

    // Check for primitive prototype methods
    // This would be called when accessing methods on primitives
    // The actual implementation would look up in env.primitivePrototypes

    return nil
}
```

### VM Integration

```go
// pkg/vm/vm_init.go

func (vm *VM) initializePrototypes() {
    // Create runtime context
    runtimeCtx := &builtins.RuntimeContext{
        VM: vm,
        DefineGlobal: func(name string, value Value) error {
            // Add to globals
            idx := len(vm.globals)
            vm.globals = append(vm.globals, value)
            vm.globalNames = append(vm.globalNames, name)
            return nil
        },
        // These will be set as we initialize
        ObjectPrototype:   Undefined,
        FunctionPrototype: Undefined,
    }

    // Initialize all builtins
    initializers := builtins.GetStandardInitializers()
    for _, init := range initializers {
        if err := init.InitRuntime(runtimeCtx); err != nil {
            fmt.Fprintf(os.Stderr, "Warning: failed to initialize %s: %v\n", init.Name(), err)
        }

        // Update context with newly created prototypes
        runtimeCtx.ObjectPrototype = vm.ObjectPrototype
        runtimeCtx.FunctionPrototype = vm.FunctionPrototype
        // ... etc
    }
}
```

## Benefits of Simplified Design

1. **No Magic**: Everything uses standard object creation and property assignment
2. **Easier to Understand**: Prototypes are just objects with properties
3. **Less Code**: No need for special registration methods
4. **More Flexible**: Can easily add complex initialization logic
5. **Better Testing**: Can create prototypes in tests the same way

## Migration Strategy

1. **Start Simple**: Begin with Object and Function initializers
2. **Parallel Operation**: Run new system alongside old during migration
3. **Incremental Migration**: Convert one builtin at a time
4. **Validate Each Step**: Ensure tests pass after each migration
5. **Clean Up**: Remove old system only after full validation

## Example: Array Initializer

```go
type ArrayInitializer struct{}

func (a *ArrayInitializer) Name() string { return "Array" }
func (a *ArrayInitializer) Priority() int { return 2 } // After Object and Function

func (a *ArrayInitializer) InitTypes(ctx *TypeContext) error {
    // Create Array.prototype type using fluent API
    arrayProtoType := types.NewObjectType().
        WithVariadicProperty("push", []types.Type{}, types.Number, types.Any).
        WithProperty("pop", types.NewSimpleFunction([]types.Type{}, types.Any)).
        WithProperty("length", types.Number).
        WithProperty("shift", types.NewSimpleFunction([]types.Type{}, types.Any)).
        WithProperty("unshift", types.NewVariadicFunction([]types.Type{}, types.Number, types.Any))
        // ... more methods can be chained

    // Create Array constructor type using fluent API
    arrayCtorType := types.NewObjectType().
        // Multiple constructor overloads
        WithSimpleCallSignature([]types.Type{}, &types.ArrayType{ElementType: types.Any}).
        WithSimpleCallSignature([]types.Type{types.Number}, &types.ArrayType{ElementType: types.Any}).
        WithVariadicCallSignature([]types.Type{}, &types.ArrayType{ElementType: types.Any}, types.Any).
        // Static methods and prototype
        WithProperty("prototype", arrayProtoType).
        WithProperty("isArray", types.NewSimpleFunction([]types.Type{types.Any}, types.Boolean))

    // Define globally
    if err := ctx.DefineGlobal("Array", arrayCtorType); err != nil {
        return err
    }

    // Store for primitive array type
    ctx.SetPrimitivePrototype("array", arrayProtoType)

    return nil
}

func (a *ArrayInitializer) InitRuntime(ctx *RuntimeContext) error {
    vm := ctx.VM

    // Create Array.prototype inheriting from Object.prototype
    arrayProto := vm.NewObject(ctx.ObjectPrototype).AsPlainObject()

    // Add all methods directly
    arrayProto.SetOwn("push", vm.NewNativeFunction(-1, true, "push", arrayPushImpl))
    arrayProto.SetOwn("pop", vm.NewNativeFunction(0, false, "pop", arrayPopImpl))
    arrayProto.SetOwn("shift", vm.NewNativeFunction(0, false, "shift", arrayShiftImpl))
    // ... more methods

    // Create Array constructor
    arrayCtor := vm.NewNativeFunction(-1, true, "Array", arrayConstructorImpl)

    // Convert to function with properties
    if ctorFunc := arrayCtor.AsNativeFunction(); ctorFunc != nil {
        ctorWithProps := vm.NewNativeFunctionWithProps(ctorFunc.Arity, ctorFunc.Variadic, ctorFunc.Name, ctorFunc.Fn)
        ctorObj := ctorWithProps.AsNativeFunctionWithProps()

        ctorObj.Props.SetOwn("prototype", arrayProto)
        ctorObj.Props.SetOwn("isArray", vm.NewNativeFunction(1, false, "isArray", arrayIsArrayImpl))

        arrayCtor = ctorWithProps
    }

    // Set constructor on prototype
    arrayProto.SetOwn("constructor", arrayCtor)

    // Store in VM
    vm.ArrayPrototype = arrayProto

    // Define globally
    return ctx.DefineGlobal("Array", arrayCtor)
}
```

## Example: Console Object (Global Object, Not Constructor)

```go
type ConsoleInitializer struct{}

func (c *ConsoleInitializer) Name() string { return "console" }
func (c *ConsoleInitializer) Priority() int { return 102 } // Low priority, non-essential

func (c *ConsoleInitializer) InitTypes(ctx *TypeContext) error {
    // Create console object type using fluent API - it's just an object with methods
    consoleType := types.NewObjectType().
        WithProperty("log", types.NewVariadicFunction([]types.Type{}, types.Void, types.Any)).
        WithProperty("error", types.NewVariadicFunction([]types.Type{}, types.Void, types.Any)).
        WithProperty("warn", types.NewVariadicFunction([]types.Type{}, types.Void, types.Any)).
        WithProperty("info", types.NewVariadicFunction([]types.Type{}, types.Void, types.Any)).
        WithProperty("debug", types.NewVariadicFunction([]types.Type{}, types.Void, types.Any)).
        WithProperty("trace", types.NewVariadicFunction([]types.Type{}, types.Void, types.Any)).
        WithProperty("clear", types.NewSimpleFunction([]types.Type{}, types.Void)).
        WithProperty("count", types.NewVariadicFunction([]types.Type{}, types.Void, types.Any)).
        WithProperty("countReset", types.NewVariadicFunction([]types.Type{}, types.Void, types.Any)).
        WithProperty("time", types.NewVariadicFunction([]types.Type{}, types.Void, types.Any)).
        WithProperty("timeEnd", types.NewVariadicFunction([]types.Type{}, types.Void, types.Any)).
        WithProperty("group", types.NewVariadicFunction([]types.Type{}, types.Void, types.Any)).
        WithProperty("groupCollapsed", types.NewVariadicFunction([]types.Type{}, types.Void, types.Any)).
        WithProperty("groupEnd", types.NewSimpleFunction([]types.Type{}, types.Void))

    // Define the console object globally (not a constructor, just an object)
    return ctx.DefineGlobal("console", consoleType)
}

func (c *ConsoleInitializer) InitRuntime(ctx *RuntimeContext) error {
    vm := ctx.VM

    // Create console object inheriting from Object.prototype
    consoleObj := vm.NewObject(ctx.ObjectPrototype).AsPlainObject()

    // Add all console methods directly using SetOwn
    consoleObj.SetOwn("log", vm.NewNativeFunction(-1, true, "log", consoleLogImpl))
    consoleObj.SetOwn("error", vm.NewNativeFunction(-1, true, "error", consoleErrorImpl))
    consoleObj.SetOwn("warn", vm.NewNativeFunction(-1, true, "warn", consoleWarnImpl))
    consoleObj.SetOwn("info", vm.NewNativeFunction(-1, true, "info", consoleInfoImpl))
    consoleObj.SetOwn("debug", vm.NewNativeFunction(-1, true, "debug", consoleDebugImpl))
    consoleObj.SetOwn("trace", vm.NewNativeFunction(-1, true, "trace", consoleTraceImpl))
    consoleObj.SetOwn("clear", vm.NewNativeFunction(0, false, "clear", consoleClearImpl))
    consoleObj.SetOwn("count", vm.NewNativeFunction(-1, true, "count", consoleCountImpl))
    consoleObj.SetOwn("countReset", vm.NewNativeFunction(-1, true, "countReset", consoleCountResetImpl))
    consoleObj.SetOwn("time", vm.NewNativeFunction(-1, true, "time", consoleTimeImpl))
    consoleObj.SetOwn("timeEnd", vm.NewNativeFunction(-1, true, "timeEnd", consoleTimeEndImpl))
    consoleObj.SetOwn("group", vm.NewNativeFunction(-1, true, "group", consoleGroupImpl))
    consoleObj.SetOwn("groupCollapsed", vm.NewNativeFunction(-1, true, "groupCollapsed", consoleGroupCollapsedImpl))
    consoleObj.SetOwn("groupEnd", vm.NewNativeFunction(0, false, "groupEnd", consoleGroupEndImpl))

    // Define globally (makes it available as global variable 'console')
    return ctx.DefineGlobal("console", consoleObj)
}
```

Note how console works:

1. **It's not a constructor** - no call signatures, just an object with methods
2. **Type definition** creates an ObjectType with method properties
3. **Runtime definition** creates a regular object and sets methods using SetOwn
4. **Global definition** makes it available as the global variable `console`

This is different from Array/Object which are constructors (callable objects with prototype property).

## Missing Methods in Types Package

You might notice the examples use `WithVariadicProperty` and `types.NewVariadicFunction` which may not exist yet. We'd need to add these to the types package:

```go
// Add to pkg/types/object.go

// WithVariadicProperty adds a variadic method property
func (ot *ObjectType) WithVariadicProperty(name string, paramTypes []types.Type, returnType Type, restType Type) *ObjectType {
    methodType := NewObjectType().WithVariadicCallSignature(paramTypes, returnType, restType)
    return ot.WithProperty(name, methodType)
}

// Add to pkg/types/types.go

// NewVariadicFunction creates a variadic function type
func NewVariadicFunction(paramTypes []Type, returnType Type, restType Type) *ObjectType {
    return NewObjectType().WithVariadicCallSignature(paramTypes, returnType, restType)
}
```

## Summary

This simplified design:

- Uses existing mechanisms (object creation, SetOwn)
- Eliminates special registration APIs
- Uses fluent API for cleaner type definitions
- Makes the code more straightforward
- Handles both constructors (Array, Object) and global objects (console) uniformly
- Maintains the same functionality with less complexity

## Detailed Implementation TODO

### Phase 1: Foundation (Week 1)

#### 1.1 Create Core Interface Types

- [ ] Create `pkg/builtins/initializer.go` with `BuiltinInitializer` interface
- [ ] Create `TypeContext` and `RuntimeContext` structs
- [ ] Create `pkg/builtins/base_initializer.go` with `InitializerList` and priority constants
- [ ] Create `pkg/builtins/standard.go` with `GetStandardInitializers()` function

#### 1.2 Add Missing Types Package Methods

- [ ] Add `WithVariadicProperty()` method to ObjectType in `pkg/types/object.go`
- [ ] Add `NewVariadicFunction()` helper to `pkg/types/types.go`
- [ ] Test the new fluent API methods work correctly

#### 1.3 Create Adapter Infrastructure

- [ ] Create `builtinTypeEnvironment` struct in `pkg/checker/environment.go`
- [ ] Create `builtinRuntimeEnvironment` struct in `pkg/vm/vm_init.go`
- [ ] Implement adapter methods for both environments

### Phase 2: Core Builtin Migration (Week 2)

#### 2.1 Object Initializer

- [ ] Create `pkg/builtins/object_init.go` with complete `ObjectInitializer`
- [ ] Implement `InitTypes()` method with Object constructor and prototype types
- [ ] Implement `InitRuntime()` method with Object constructor and prototype objects
- [ ] Implement all Object static methods (create, keys, getPrototypeOf, setPrototypeOf)
- [ ] Implement all Object.prototype methods (hasOwnProperty, toString, valueOf, isPrototypeOf)
- [ ] Test Object initializer works in isolation

#### 2.2 Function Initializer

- [ ] Create `pkg/builtins/function_init.go` with complete `FunctionInitializer`
- [ ] Implement Function constructor type and runtime
- [ ] Implement Function.prototype methods (call, apply, bind)
- [ ] Test Function initializer works with Object as base

#### 2.3 Array Initializer

- [ ] Create `pkg/builtins/array_init.go` with complete `ArrayInitializer`
- [ ] Migrate all array prototype methods from existing `pkg/builtins/array.go`
- [ ] Implement Array constructor overloads (empty, length, ...items)
- [ ] Implement Array static methods (isArray, from)
- [ ] Test Array initializer works with Object and Function as bases

### Phase 3: Cut-Over Integration (Week 3)

**This is the "big bang" phase - we completely replace the old system**

#### 3.1 Complete System Replacement

- [ ] **REMOVE** old builtin registry system from `pkg/builtins/builtins.go`
- [ ] **REMOVE** old VM callback system from `pkg/builtins/vm_integration.go`
- [ ] **REPLACE** `NewGlobalEnvironment()` to use new initializer system
- [ ] **REPLACE** `initializePrototypes()` to use new initializer system
- [ ] **UPDATE** `getBuiltinType()` method to use primitive prototype registry

#### 3.2 Integration Testing (All-or-Nothing)

- [ ] Test basic REPL functionality works (if this fails, immediate rollback)
- [ ] Test all core builtin types resolve correctly (Object, Function, Array)
- [ ] Test all core builtin runtime objects work (constructors, methods)
- [ ] Fix any immediate issues found (time-boxed to 1 day)

#### 3.3 Compiler/Global Integration

- [ ] Ensure compiler can resolve global builtin references
- [ ] Update global variable index mapping to work with new system
- [ ] Test compiled code can access all builtins correctly

**Critical Success Metric**: REPL must work with Object, Function, Array after this phase, or we rollback immediately.

### Phase 4: Remaining Builtins (Week 4)

#### 4.1 String Initializer

- [ ] Create `pkg/builtins/string_init.go`
- [ ] Migrate all string methods from existing `pkg/builtins/string.go`
- [ ] Test string primitive method access works

#### 4.2 Number and Boolean Initializers

- [ ] Create `pkg/builtins/number_init.go`
- [ ] Create `pkg/builtins/boolean_init.go`
- [ ] Implement primitive boxing/unboxing if needed

#### 4.3 Utility Object Initializers

- [ ] Create `pkg/builtins/math_init.go` (migrate from `pkg/builtins/math.go`)
- [ ] Create `pkg/builtins/json_init.go` (migrate from `pkg/builtins/json.go`)
- [ ] Create `pkg/builtins/console_init.go` (migrate from `pkg/builtins/console.go`)
- [ ] Create `pkg/builtins/date_init.go` (migrate from `pkg/builtins/date.go`)

### Phase 5: Cleanup and Validation (Week 5)

#### 5.1 Remove Old System

- [ ] Delete old registry functions from `pkg/builtins/builtins.go`
- [ ] Delete `pkg/builtins/vm_integration.go`
- [ ] Remove old builtin registration calls
- [ ] Clean up any remaining adapter code

#### 5.2 Testing and Documentation

- [ ] Run full test suite and fix any regressions
- [ ] Add comprehensive tests for new initializer system
- [ ] Test custom builtin registration (extensibility)
- [ ] Update documentation and examples
- [ ] Performance test to ensure no regressions

#### 5.3 Final Validation

- [ ] Manual REPL testing of all builtins
- [ ] Test all TypeScript examples from test suite
- [ ] Verify error messages are appropriate
- [ ] Check memory usage hasn't increased

### Rollback Plan

**Philosophy: Git is our safety net - no parallel systems**

We will NOT run old and new systems in parallel. This approach previously caused interference between global prototypes and VM callback systems, creating more problems than it solved.

#### Preparation

- [ ] Create feature branch for refactor
- [ ] Tag current working state before starting
- [ ] Ensure all tests pass on main branch
- [ ] Document current system behavior for reference

#### Rollback Triggers

- [ ] Fundamental architecture issue discovered that invalidates the approach

#### Rollback Steps

1. [ ] Fix issues or redesign approach
2. [ ] Retry when ready

**Key insight**: Clean breaks are better than messy parallel systems. If we hit a wall, we will consider reverting.

### Success Criteria

#### Functional Requirements

- [ ] All existing tests pass
- [ ] REPL works identically to before
- [ ] All builtin types are available to checker
- [ ] All builtin runtime objects work correctly
- [ ] Error messages are equivalent or better

#### Non-Functional Requirements

- [ ] Performance is same or better
- [ ] Memory usage is same or better
- [ ] Code is more maintainable (subjective but measurable via metrics)
- [ ] New builtins can be added easily
- [ ] Custom user builtins are possible

#### Quality Gates

- [ ] Code review approval
- [ ] All tests passing
- [ ] Performance benchmarks meet criteria
- [ ] Documentation updated
- [ ] Migration guide written

### Risk Mitigation

#### High Risk Items

1. **VM prototype chain initialization order** - Test thoroughly with Object → Function → Array chain
2. **Global variable indexing** - Ensure compiler's global mapping stays consistent
3. **Checker type resolution** - Verify all builtin types resolve correctly
4. **Method binding on primitives** - Test string.charAt(), array.push(), etc.

#### Medium Risk Items

1. **Memory management** - Monitor for prototype object leaks
2. **Error handling** - Ensure initialization errors are handled gracefully
3. **Extensibility** - Test that custom builtins work as designed

#### Contingency Plans

- If prototype chain is broken during Phase 3: **Investigate and fix**
- If global indexing breaks: **Quick fix if < 4 hours, otherwise revert**
- If type resolution fails: **Analyze why initializers aren't working**
- If method binding breaks: **Check if it's initialization order issue, fix**

**Philosophy**: No band-aids or fallbacks. If it doesn't work cleanly, fix the root cause.

### Post-Implementation

#### Monitoring

- [ ] Set up performance monitoring
- [ ] Monitor memory usage in production
- [ ] Track error rates and types

#### Future Enhancements

- [ ] Consider adding builtin versioning
- [ ] Add plugin system for user-defined builtins
- [ ] Optimize initialization performance if needed
- [ ] Add hot-reloading of builtin definitions for development

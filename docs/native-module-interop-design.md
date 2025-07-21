# Native Module Interop Design

## Overview

This document outlines the design for allowing Go code to declare modules that can be imported in TypeScript using normal `import` statements. The system provides a declarative API for exposing Go functions, types, and values to the TypeScript runtime with automatic type generation.

## Goals

1. **Simple Declarative API**: Easy-to-use Go API for module declaration
2. **Lazy Initialization**: Modules initialize only when first imported
3. **Single Instance**: Each module initializes exactly once per Paserati session
4. **Automatic Type Generation**: Use Go reflection to generate TypeScript types
5. **Seamless Integration**: Works with existing module system and import statements
6. **Type Safety**: Full TypeScript type checking support

## API Design

### Module Declaration API

```go
// Main module declaration function
func (p *Paserati) DeclareModule(name string, builder func(m *ModuleBuilder)) *NativeModule

// ModuleBuilder provides the declarative API
type ModuleBuilder struct {
    // Constants and variables
    Const(name string, value interface{}) *ModuleBuilder
    Let(name string, value interface{}) *ModuleBuilder
    Var(name string, value interface{}) *ModuleBuilder
    
    // Functions
    Function(name string, fn interface{}) *ModuleBuilder
    AsyncFunction(name string, fn interface{}) *ModuleBuilder
    
    // Classes and constructors
    Class(name string, goStruct interface{}) *ModuleBuilder
    Constructor(name string, fn interface{}) *ModuleBuilder
    
    // Namespaces for grouping
    Namespace(name string, builder func(ns *NamespaceBuilder)) *ModuleBuilder
    
    // Type definitions
    Interface(name string, fields map[string]interface{}) *ModuleBuilder
    Type(name string, typedef interface{}) *ModuleBuilder
    
    // Default export
    Default(value interface{}) *ModuleBuilder
}
```

### Usage Examples

```go
// Example 1: Math utilities module
mathModule := paserati.DeclareModule("math-utils", func(m *ModuleBuilder) {
    m.Const("PI_SQUARED", math.Pi * math.Pi)
    m.Function("square", func(x float64) float64 { return x * x })
    m.Function("factorial", factorial)
    m.Default(map[string]interface{}{
        "square": func(x float64) float64 { return x * x },
        "factorial": factorial,
    })
})

// Example 2: File system operations
fsModule := paserati.DeclareModule("fs", func(m *ModuleBuilder) {
    m.AsyncFunction("readFile", readFileAsync)
    m.Function("existsSync", func(path string) bool {
        _, err := os.Stat(path)
        return err == nil
    })
    m.Class("FileInfo", (*os.FileInfo)(nil))
})

// Example 3: Custom data structures
dataModule := paserati.DeclareModule("collections", func(m *ModuleBuilder) {
    m.Constructor("HashMap", NewHashMap)
    m.Interface("Hashable", map[string]interface{}{
        "hash": "() => number",
        "equals": "(other: any) => boolean",
    })
})

// Then in TypeScript:
// import { square, factorial } from "math-utils";
// import * as fs from "fs"; 
// import { HashMap } from "collections";
```

## Architecture Components

### 1. NativeModule Structure

```go
type NativeModule struct {
    name         string
    builder      func(*ModuleBuilder)
    initialized  bool
    exports      map[string]vm.Value
    typeExports  map[string]types.Type
    mutex        sync.Once
}
```

### 2. NativeModuleResolver

```go
type NativeModuleResolver struct {
    modules map[string]*NativeModule
    priority int
}

func (r *NativeModuleResolver) CanResolve(specifier string) bool {
    _, exists := r.modules[specifier]
    return exists
}

func (r *NativeModuleResolver) Resolve(specifier, fromPath string) (*ResolvedModule, error) {
    module := r.modules[specifier]
    return &ResolvedModule{
        Specifier: specifier,
        ResolvedPath: "native://" + specifier,
        Source: &NativeModuleSource{module: module},
        Resolver: "native",
    }, nil
}
```

### 3. Reflection-Based Type Generation

```go
type TypeGenerator struct {
    typeCache map[reflect.Type]types.Type
}

func (tg *TypeGenerator) GenerateType(goValue interface{}) types.Type {
    t := reflect.TypeOf(goValue)
    
    switch t.Kind() {
    case reflect.Func:
        return tg.generateFunctionType(t)
    case reflect.Struct:
        return tg.generateStructType(t)
    case reflect.Interface:
        return tg.generateInterfaceType(t)
    // ... other kinds
    }
}

func (tg *TypeGenerator) generateFunctionType(t reflect.Type) types.Type {
    params := make([]types.Type, t.NumIn())
    for i := 0; i < t.NumIn(); i++ {
        params[i] = tg.mapGoTypeToTS(t.In(i))
    }
    
    var returnType types.Type = types.Void
    if t.NumOut() > 0 {
        returnType = tg.mapGoTypeToTS(t.Out(0))
    }
    
    return types.NewSimpleFunction(params, returnType)
}
```

### 4. Value Conversion System

```go
type ValueConverter struct {
    vm *vm.VM
}

func (vc *ValueConverter) ConvertToVM(goValue interface{}) vm.Value {
    switch v := goValue.(type) {
    case func(...interface{}) interface{}:
        return vc.wrapGoFunction(v)
    case string:
        return vm.NewString(v)
    case float64:
        return vm.NewNumber(v)
    // ... other types
    }
}

func (vc *ValueConverter) wrapGoFunction(fn interface{}) vm.Value {
    return &vm.NativeFunction{
        Name: "native_function",
        Call: func(args []vm.Value) (vm.Value, error) {
            // Convert VM values to Go values
            goArgs := make([]interface{}, len(args))
            for i, arg := range args {
                goArgs[i] = vc.convertFromVM(arg)
            }
            
            // Call Go function using reflection
            fnValue := reflect.ValueOf(fn)
            results := fnValue.Call(vc.makeReflectValues(goArgs))
            
            // Convert result back to VM value
            if len(results) > 0 {
                return vc.ConvertToVM(results[0].Interface()), nil
            }
            return vm.Undefined, nil
        },
    }
}
```

## Integration Points

### 1. Paserati Driver Integration

```go
func (p *Paserati) DeclareModule(name string, builder func(*ModuleBuilder)) *NativeModule {
    module := &NativeModule{
        name: name,
        builder: builder,
    }
    
    // Add to native resolver if not exists
    if p.nativeResolver == nil {
        p.nativeResolver = NewNativeModuleResolver()
        p.moduleLoader.AddResolver(p.nativeResolver)
    }
    
    p.nativeResolver.RegisterModule(name, module)
    return module
}
```

### 2. Module Loading Integration

The native modules integrate seamlessly with the existing module loading system:

1. **Resolution Phase**: `NativeModuleResolver` checks if specifier matches declared module
2. **Loading Phase**: Returns synthetic `ResolvedModule` with native source
3. **Parsing Phase**: Generates synthetic AST with export declarations
4. **Type Checking**: Uses generated TypeScript types for full type checking
5. **Compilation**: Generates bytecode that calls native functions
6. **Runtime**: VM executes with proper Go function wrappers

### 3. Lazy Initialization

```go
func (nm *NativeModule) ensureInitialized(vm *vm.VM) {
    nm.mutex.Do(func() {
        builder := &ModuleBuilder{
            exports: make(map[string]vm.Value),
            typeExports: make(map[string]types.Type),
            converter: NewValueConverter(vm),
            typeGen: NewTypeGenerator(),
        }
        
        nm.builder(builder)
        
        nm.exports = builder.exports
        nm.typeExports = builder.typeExports
        nm.initialized = true
    })
}
```

## Advanced Features

### 1. Async Function Support

```go
func (m *ModuleBuilder) AsyncFunction(name string, fn interface{}) *ModuleBuilder {
    // Wrap function to return Promise
    wrapper := func(args []vm.Value) vm.Value {
        promise := vm.NewPromise()
        
        go func() {
            result, err := callGoFunction(fn, args)
            if err != nil {
                promise.Reject(err)
            } else {
                promise.Resolve(result)
            }
        }()
        
        return promise
    }
    
    return m.Function(name, wrapper)
}
```

### 2. Class Integration

```go
func (m *ModuleBuilder) Class(name string, goStruct interface{}) *ModuleBuilder {
    structType := reflect.TypeOf(goStruct)
    
    // Generate constructor function
    constructor := func(args []vm.Value) vm.Value {
        instance := reflect.New(structType.Elem()).Interface()
        return m.converter.ConvertToVM(instance)
    }
    
    // Generate method bindings
    methods := m.generateMethodBindings(structType)
    
    return m.Constructor(name, constructor).
           Namespace(name, func(ns *NamespaceBuilder) {
               for methodName, method := range methods {
                   ns.Function(methodName, method)
               }
           })
}
```

### 3. Error Handling

```go
type NativeError struct {
    Message string
    Code    string
    Cause   error
}

func (ne *NativeError) Error() string {
    return ne.Message
}

func (m *ModuleBuilder) Function(name string, fn interface{}) *ModuleBuilder {
    wrapped := func(args []vm.Value) (vm.Value, error) {
        defer func() {
            if r := recover(); r != nil {
                // Convert Go panic to TypeScript Error
                panic(&NativeError{
                    Message: fmt.Sprintf("Native function %s panicked: %v", name, r),
                    Code: "NATIVE_PANIC",
                })
            }
        }()
        
        return callGoFunction(fn, args)
    }
    
    // ... rest of implementation
}
```

## Implementation Plan

1. **Phase 1**: Basic module declaration API and native resolver
2. **Phase 2**: Reflection-based type generation for simple types
3. **Phase 3**: Function wrapping and value conversion
4. **Phase 4**: Advanced features (async, classes, error handling)
5. **Phase 5**: Performance optimization and caching

## Benefits

1. **Developer Experience**: Simple, declarative API for exposing Go functionality
2. **Type Safety**: Full TypeScript type checking with auto-generated types
3. **Performance**: Direct Go function calls without serialization overhead  
4. **Integration**: Works seamlessly with existing module system
5. **Flexibility**: Supports functions, classes, constants, and complex types
6. **Extensibility**: Easy to add new conversion rules and advanced features
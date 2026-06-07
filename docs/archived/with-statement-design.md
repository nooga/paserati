# With Statement Implementation Design

## Overview

This document outlines the design for implementing JavaScript's `with` statement in Paserati using a compile-time lowering approach. The key principle is to resolve property access at compile time and emit `OpGetProp`/`OpSetProp` bytecode instructions, avoiding runtime scope manipulation.

## Background

The `with` statement in JavaScript extends the scope chain with the properties of an object:

```javascript
with (obj) {
    x = 42;  // If obj has property 'x', sets obj.x = 42
             // Otherwise, accesses outer scope variable 'x'
}
```

## Design Principles

1. **No Runtime Scope Pushing**: All property resolution happens at compile time
2. **Reuse Existing Infrastructure**: Leverage existing environment stacks in checker and compiler
3. **Minimal Intrusion**: Extend existing systems without major architectural changes
4. **Type Safety**: Maintain TypeScript's type checking capabilities

## Implementation Strategy

### Phase 1: Type Checker Extensions

The type checker already has an `Environment` structure that tracks variable types. We'll extend it to track "with objects":

```go
// In pkg/checker/environment.go
type Environment struct {
    // ... existing fields ...
    
    // NEW: Track objects in with statements
    withObjects []WithObject  // Stack of objects from enclosing with statements
}

type WithObject struct {
    ExprType    types.Type  // Type of the with expression
    Properties  map[string]types.Type  // Known properties and their types
}
```

When checking a `with` statement:
1. Evaluate the with expression to get its type
2. Extract known properties from the type (if it's an object type)
3. Push a `WithObject` entry onto the environment's stack
4. Check the body with this extended environment
5. Pop the `WithObject` when done

### Phase 2: Compiler Extensions

The compiler's `SymbolTable` tracks variable locations. We'll extend it to track with objects:

```go
// In pkg/compiler/symbol_table.go
type SymbolTable struct {
    // ... existing fields ...
    
    // NEW: Track with object registers
    withObjects []WithObjectInfo  // Stack of with objects
}

type WithObjectInfo struct {
    ObjectRegister Register  // Register containing the with object
    Properties     map[string]bool  // Set of known properties
}
```

### Phase 3: Resolution Logic

#### Type Checker Resolution

When resolving an identifier in the type checker:

```go
func (e *Environment) ResolveWithFallback(name string) (types.Type, bool, bool) {
    // First try normal variable resolution
    if typ, _, found := e.Resolve(name); found {
        return typ, false, true  // Found as variable, not from with
    }
    
    // Then check with objects from innermost to outermost
    for i := len(e.withObjects) - 1; i >= 0; i-- {
        withObj := e.withObjects[i]
        if propType, exists := withObj.Properties[name]; exists {
            return propType, true, true  // Found as with property
        }
    }
    
    return nil, false, false  // Not found
}
```

#### Compiler Resolution

When compiling an identifier reference:

```go
func (c *Compiler) compileIdentifier(node *parser.Identifier) Register {
    // First try normal symbol resolution
    if symbol, _, found := c.currentSymbolTable.Resolve(node.Value); found {
        // Normal variable access
        return c.loadSymbol(symbol)
    }
    
    // Then check with objects
    if withReg, found := c.resolveWithProperty(node.Value); found {
        // Emit property access bytecode
        propName := c.chunk.AddConstant(vm.NewVMString(node.Value))
        resultReg := c.regAlloc.Allocate()
        c.emit(OpGetProp, byte(resultReg), byte(withReg), propName)
        return resultReg
    }
    
    // Error: undefined variable
    c.addError(node, "undefined variable: " + node.Value)
}
```

### Phase 4: Assignment Handling

For assignments within a with block, we need special handling:

```go
func (c *Compiler) compileAssignment(left parser.Expression, valueReg Register) {
    if ident, ok := left.(*parser.Identifier); ok {
        // Check if it's a with property
        if withReg, found := c.resolveWithProperty(ident.Value); found {
            // Emit property setter
            propName := c.chunk.AddConstant(vm.NewVMString(ident.Value))
            c.emit(OpSetProp, byte(withReg), byte(valueReg), propName)
            return
        }
    }
    
    // Fall back to normal assignment handling
    // ...
}
```

### Phase 5: Edge Cases

1. **Dynamic Properties**: If the with object's type is `any` or unknown, we can't determine properties at compile time. In this case:
   - Type checker: Allow any property access but type it as `any`
   - Compiler: Still emit `OpGetProp`/`OpSetProp` for all unresolved identifiers

2. **Nested With Statements**: The stack-based approach naturally handles nesting - inner with objects are checked first.

3. **Property Shadowing**: Properties from inner with objects shadow those from outer ones and regular variables.

4. **Getters/Setters**: The VM's `OpGetProp` and `OpSetProp` already handle getters/setters correctly.

## Implementation Steps

1. **Extend Type Checker Environment** (pkg/checker/environment.go)
   - Add `withObjects` field
   - Add `PushWithObject` and `PopWithObject` methods
   - Modify `Resolve` to check with objects

2. **Update Type Checker Statement Handler** (pkg/checker/statements.go)
   - Implement proper `checkWithStatement`
   - Push/pop with objects around body checking

3. **Extend Compiler Symbol Table** (pkg/compiler/symbol_table.go)
   - Add `withObjects` field
   - Add methods for with object management

4. **Update Compiler** (pkg/compiler/compiler.go)
   - Add `resolveWithProperty` method
   - Modify identifier compilation
   - Modify assignment compilation

5. **Handle With Statement Compilation** (pkg/compiler/compile_statements.go)
   - Compile the with expression to get object register
   - Push with object info
   - Compile body
   - Pop with object info

## Testing Strategy

Use the existing test files in `tests/scripts/`:
- `with_basic.ts` - Basic property access
- `with_assignment.ts` - Property assignment
- `with_nested.ts` - Nested with statements
- `with_getter_setter.ts` - Getter/setter invocation

## Benefits of This Approach

1. **No Runtime Overhead**: All resolution happens at compile time
2. **Type Safety**: Maintains full type checking for known properties
3. **Compatibility**: Works with existing VM opcodes
4. **Simplicity**: Extends existing systems rather than creating new ones
5. **Performance**: No dynamic scope chain manipulation at runtime

## Alternative Considered

The runtime approach (pushing scope objects) was considered but rejected because:
- It would require new VM opcodes for scope manipulation
- It would add runtime overhead for every variable access
- It conflicts with Paserati's compile-time philosophy
- It would complicate the VM's execution model
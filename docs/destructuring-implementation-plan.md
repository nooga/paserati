# Destructuring Assignment Implementation Plan

This document outlines a comprehensive plan for implementing destructuring assignment in Paserati, following a phased approach that builds incrementally on existing VM capabilities.

## Overview

Destructuring assignment allows unpacking values from arrays or properties from objects into distinct variables. The implementation strategy is to **desugar** destructuring syntax into a series of simpler operations (property access, array indexing, assignments) that the VM already supports.

## Phase 1: Basic Array Destructuring in Assignments ✅ COMPLETED

### Goal
Implement basic array destructuring assignments like:
```typescript
let [a, b, c] = [1, 2, 3];
let [x, y] = someArray;
```

### Implementation Steps

#### 1.1 AST Extensions (`pkg/parser/ast.go`) ✅ COMPLETED
```go
// ArrayDestructuringAssignment represents [a, b, c] = expr
type ArrayDestructuringAssignment struct {
    BaseExpression                    // Embed base for ComputedType
    Token          lexer.Token        // The '[' token
    Elements       []*DestructuringElement  // Target variables/patterns
    Value          Expression         // RHS expression to destructure
}

// DestructuringElement represents a single element in destructuring pattern
type DestructuringElement struct {
    Target   Expression  // Target variable (Identifier for now)
    Default  Expression  // Default value (nil if no default)
}

func (ada *ArrayDestructuringAssignment) expressionNode() {}
func (ada *ArrayDestructuringAssignment) TokenLiteral() string { return ada.Token.Literal }
func (ada *ArrayDestructuringAssignment) String() string {
    // Implementation for debugging
}
```

#### 1.2 Lexer Extensions (`pkg/lexer/lexer.go`) ✅ COMPLETED
No new tokens needed - reuse existing `[`, `]`, `=`, `,` tokens.

#### 1.3 Parser Extensions (`pkg/parser/parser.go`) ✅ COMPLETED
- ✅ Detect destructuring patterns in assignment contexts
- ✅ Add parsing logic to distinguish between array literals and destructuring patterns
- ✅ Handle parsing of `[a, b, c] = expr` syntax

```go
// In parseAssignmentExpression or similar:
func (p *Parser) parseArrayDestructuringAssignment() *ArrayDestructuringAssignment {
    // Parse [element1, element2, ...] = value pattern
}

// Helper to parse destructuring elements
func (p *Parser) parseDestructuringElement() *DestructuringElement {
    // Parse target variable (identifier for now)
}
```

#### 1.4 Type Checker Extensions (`pkg/checker/`) ✅ COMPLETED
- ✅ Validate that RHS is array-like (has numeric indices)
- ✅ Check element count compatibility 
- ✅ Infer types for destructured variables from array element types
- ✅ Handle tuple types specially for precise type checking

```go
func (c *Checker) checkArrayDestructuringAssignment(node *ArrayDestructuringAssignment) {
    // 1. Check RHS is array-like type
    // 2. Validate element assignments are type-compatible
    // 3. Set computed types for target variables
}
```

#### 1.5 Compiler Extensions (`pkg/compiler/`) ✅ COMPLETED
**Core Strategy: Desugar into simple operations**

```go
func (c *Compiler) compileArrayDestructuringAssignment(node *ArrayDestructuringAssignment, hint Register) (Register, errors.PaseratiError) {
    // 1. Compile RHS expression into temp register
    tempReg := c.regAlloc.Alloc()
    defer c.regAlloc.Free(tempReg)
    _, err := c.compileNode(node.Value, tempReg)
    
    // 2. For each element, compile: target = temp[index]
    for i, element := range node.Elements {
        indexReg := c.regAlloc.Alloc()
        c.emitLoadConst(indexReg, i, node.Token.Line)  // Load index
        
        valueReg := c.regAlloc.Alloc()
        c.emitGetIndex(valueReg, tempReg, indexReg, node.Token.Line)  // Get temp[i]
        
        // Assign to target variable
        c.compileAssignment(element.Target, valueReg)
        
        c.regAlloc.Free(indexReg)
        c.regAlloc.Free(valueReg)
    }
    
    // Return the original RHS value (like regular assignment)
    c.emitMove(hint, tempReg, node.Token.Line)
    return hint, nil
}
```

#### 1.6 VM Extensions (`pkg/vm/`) ✅ COMPLETED
No new opcodes needed! Uses existing:
- ✅ `OpGetIndex` for array element access
- ✅ `OpSetGlobal`/`OpSetLocal` for variable assignment
- ✅ `OpMove` for register operations

### Test Cases for Phase 1 ✅ COMPLETED
```typescript
// Basic destructuring
let [a, b] = [1, 2];
// a === 1, b === 2

// More elements than array
let [x, y, z] = [10, 20];
// x === 10, y === 20, z === undefined

// Array variable
let arr = [1, 2, 3];
let [first, second] = arr;
// first === 1, second === 2

// Type checking
let [num, str]: [number, string] = [42, "hello"];

// Nested arrays (simple)
let [a, b] = [[1, 2], [3, 4]];
// a === [1, 2], b === [3, 4]
```

## Phase 2: Basic Object Destructuring in Assignments

### Goal
Implement basic object destructuring assignments:
```typescript
let {name, age} = person;
let {x, y} = point;
```

### Implementation Steps

#### 2.1 AST Extensions
```go
// ObjectDestructuringAssignment represents {a, b} = expr
type ObjectDestructuringAssignment struct {
    BaseExpression                    // Embed base for ComputedType
    Token          lexer.Token        // The '{' token
    Properties     []*DestructuringProperty
    Value          Expression         // RHS expression to destructure
}

// DestructuringProperty represents key: target in destructuring
type DestructuringProperty struct {
    Key     *Identifier    // Property name in source object
    Target  Expression     // Target variable (can be different from key)
    Default Expression     // Default value (nil if no default)
}
```

#### 2.2 Parser Extensions
- Parse `{prop1, prop2} = obj` syntax
- Handle shorthand `{name}` vs explicit `{name: localVar}` syntax
- Distinguish from object literals in assignment context

#### 2.3 Compiler Strategy
```go
func (c *Compiler) compileObjectDestructuringAssignment(node *ObjectDestructuringAssignment, hint Register) (Register, errors.PaseratiError) {
    // 1. Compile RHS into temp register
    tempReg := c.regAlloc.Alloc()
    defer c.regAlloc.Free(tempReg)
    _, err := c.compileNode(node.Value, tempReg)
    
    // 2. For each property: target = temp.propertyName
    for _, prop := range node.Properties {
        valueReg := c.regAlloc.Alloc()
        
        // Get property from object
        propNameIdx := c.chunk.AddConstant(vm.String(prop.Key.Value))
        c.emitGetProp(valueReg, tempReg, propNameIdx, node.Token.Line)
        
        // Assign to target
        c.compileAssignment(prop.Target, valueReg)
        
        c.regAlloc.Free(valueReg)
    }
    
    return hint, nil
}
```

### Test Cases for Phase 2
```typescript
// Basic object destructuring
let {name, age} = {name: "John", age: 30};
// name === "John", age === 30

// Different target names
let {name: firstName, age: years} = person;

// Missing properties
let {x, y, z} = {x: 1, y: 2};
// x === 1, y === 2, z === undefined

// Type checking with interfaces
interface Point { x: number; y: number; }
let {x, y}: Point = getPoint();
```

## Phase 3: Destructuring with Defaults

### Goal
Add support for default values:
```typescript
let [a = 5, b = 10] = arr;
let {name = "Unknown", age = 0} = person;
```

### Implementation Strategy
Desugar defaults using conditional expressions:
```typescript
// [a = 5, b = 10] = arr becomes:
let temp = arr;
let a = temp[0] !== undefined ? temp[0] : 5;
let b = temp[1] !== undefined ? temp[1] : 10;
```

### Compiler Extensions
```go
// In array destructuring compilation:
if element.Default != nil {
    // Compile: target = (temp[i] !== undefined) ? temp[i] : default
    c.compileConditionalAssignment(element.Target, valueReg, element.Default)
} else {
    // Simple assignment
    c.compileAssignment(element.Target, valueReg)
}
```

## Phase 4: Rest Elements in Arrays

### Goal
Support rest syntax in array destructuring:
```typescript
let [first, ...rest] = arr;
let [a, b, ...others] = [1, 2, 3, 4, 5];
```

### Implementation Requirements
- Need new VM operation for array slicing
- Add `OpSliceArray` or use existing array methods

### Compiler Strategy
```typescript
// [first, ...rest] = arr becomes:
let temp = arr;
let first = temp[0];
let rest = temp.slice(1);  // New array operation needed
```

## Phase 5: Function Parameter Destructuring

### Goal
Support destructuring in function parameters:
```typescript
function process([x, y]: [number, number]) { ... }
function greet({name, age}: Person) { ... }
```

### Implementation Strategy
Transform destructuring parameters into regular parameters + destructuring assignments:

```typescript
// function f([a, b]) { body } becomes:
function f(param0) {
    let [a, b] = param0;  // Use existing destructuring assignment
    // ... original body
}
```

### Parser Extensions
- Modify function parameter parsing to recognize destructuring patterns
- Generate appropriate AST with parameter transformation

## Phase 6: Advanced Features

### 6.1 Nested Destructuring
```typescript
let [a, [b, c]] = [1, [2, 3]];
let {user: {name, age}} = data;
```

### 6.2 Mixed Destructuring
```typescript
let {users: [first, second]} = response;
```

### 6.3 Computed Property Names
```typescript
let {[key]: value} = obj;
```

## Implementation Order Summary

1. **Phase 1**: Basic array destructuring assignments ✅ **COMPLETED**
2. **Phase 2**: Basic object destructuring assignments  
3. **Phase 3**: Default values
4. **Phase 4**: Rest elements  
5. **Phase 5**: Function parameters
6. **Phase 6**: Advanced nested patterns

## Testing Strategy

### Unit Tests
- Parser tests for correct AST generation
- Type checker tests for validation
- Compiler tests for correct bytecode generation

### Integration Tests  
- Runtime behavior tests in `tests/scripts/`
- Type checking integration
- Error message validation

### Test Files Structure
```
tests/scripts/destructuring/
├── array_basic.ts ✅ COMPLETED  
├── array_defaults.ts  
├── array_rest.ts
├── object_basic.ts
├── object_defaults.ts
├── function_params.ts
├── nested_patterns.ts
└── error_cases.ts
```

## Error Handling

### Compile-Time Errors
- Invalid destructuring patterns
- Type mismatches between pattern and value
- Invalid default value types

### Runtime Behavior
- Missing properties/elements result in `undefined` (JavaScript semantics)
- Non-iterable values in array destructuring should throw TypeError
- Non-object values in object destructuring should throw TypeError

## Notes

- **No new opcodes required** for basic functionality - leverages existing array indexing and property access
- **Follows JavaScript semantics** for undefined handling and type coercion
- **TypeScript compatibility** maintained throughout implementation  
- **Incremental approach** allows testing and validation at each phase
- **Desugaring strategy** keeps VM complexity low while providing high-level language features

This plan provides a clear roadmap for implementing destructuring assignment while building incrementally on Paserati's existing robust foundation.

---

## ✅ Phase 1 Completion Summary

**Date Completed:** December 2024

**What Was Implemented:**
- ✅ **AST Extensions**: Added `ArrayDestructuringAssignment` and `DestructuringElement` nodes
- ✅ **Parser Integration**: Detects `[a, b, c] = expr` patterns and converts array literals to destructuring in assignment contexts
- ✅ **Type Checker Support**: Validates array-like RHS types, handles tuples with precise type checking, infers variable types
- ✅ **Compiler Implementation**: Desugars destructuring to simple `OpGetIndex` operations using existing VM infrastructure
- ✅ **Testing Suite**: Created `destructuring_array_basic.ts` with comprehensive test cases

**Key Technical Achievements:**
- **Zero new VM opcodes** required - leverages existing `OpGetIndex`, `OpSetGlobal`, `OpMove`
- **Efficient bytecode generation**: `[a,b,c] = [1,2,3]` compiles to optimal index operations
- **Full TypeScript compatibility** with proper type inference and error reporting
- **Robust error handling** for malformed patterns and type mismatches

**Supported Syntax:**
```typescript
// ✅ Basic destructuring
let a, b, c;
[a, b, c] = [1, 2, 3];

// ✅ Variable sources  
let arr = [10, 20];
[x, y] = arr;

// ✅ More targets than elements (extras become undefined)
[p, q, r] = [100, 200]; // r === undefined

// ✅ Nested arrays as elements
[arr1, arr2] = [[1, 2], [3, 4]];
```

**Ready for Phase 2:** The foundation is now in place to implement object destructuring following the same desugaring strategy.
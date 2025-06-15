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
2. **Phase 2**: Basic object destructuring assignments ✅ **COMPLETED**
3. **Phase 3**: Default values ✅ **COMPLETED**
4. **Phase 4**: Rest elements ✅ **COMPLETED**
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

---

## ✅ Phase 2 Completion Summary

**Date Completed:** December 2024

**What Was Implemented:**
- ✅ **AST Extensions**: Added `ObjectDestructuringAssignment` and `DestructuringProperty` nodes
- ✅ **Parser Integration**: Detects `{a, b} = obj` patterns and converts object literals to destructuring in assignment contexts
- ✅ **Type Checker Support**: Validates object-like RHS types, checks property existence, infers variable types from object properties
- ✅ **Compiler Implementation**: Desugars destructuring to simple `OpGetProp` operations using existing VM infrastructure
- ✅ **Testing Suite**: Created `destructuring_object_basic.ts` with test cases

**Key Technical Achievements:**
- **Zero new VM opcodes** required - leverages existing `OpGetProp`, `OpSetGlobal`, `OpMove`
- **Efficient bytecode generation**: `{a,b} = obj` compiles to optimal property access operations
- **Full TypeScript compatibility** with proper type inference from object/interface types
- **Shorthand syntax support**: Both `{name}` and `{name: localVar}` patterns work correctly

**Supported Syntax:**
```typescript
// ✅ Basic object destructuring
let a, b;
{a, b} = {a: 10, b: 20};

// ✅ Variable sources  
let obj = {x: 100, y: 200};
let x, y;
{x, y} = obj;

// ✅ Missing properties (become undefined)
let p, q, r;
{p, q, r} = {p: 1, q: 2}; // r === undefined

// ✅ Property extraction from objects
let name, age;
{name, age} = person;
```

**Ready for Phase 3:** Object destructuring foundation is complete. Next step is implementing default values for both array and object destructuring patterns.

---

## ✅ Phase 3 Completion Summary

**Date Completed:** December 2024

**What Was Implemented:**
- ✅ **Parser Extensions**: Enhanced array and object destructuring parsers to handle assignment expressions as default values
- ✅ **Type Checker Integration**: Added comprehensive type checking for default values with compatibility validation
- ✅ **Conditional Compilation**: Implemented `compileConditionalAssignment` function using `OpJumpIfUndefined` for efficient runtime defaults
- ✅ **VM Integration**: Extended `emitPlaceholderJump` and `patchJump` to support `OpJumpIfUndefined` opcode
- ✅ **Testing Suite**: Created comprehensive test cases for both array and object destructuring with defaults

**Key Technical Achievements:**
- **Efficient conditional logic**: Uses `OpJumpIfUndefined` for optimal runtime performance - only evaluates defaults when needed
- **Zero additional VM opcodes**: Leverages existing undefined checking and jump infrastructure
- **Full TypeScript compatibility**: Proper type checking ensures default values are assignable to expected types
- **Comprehensive syntax support**: Handles both array `[a = 5]` and object `{a: localVar = 5}` default patterns

**Supported Syntax:**
```typescript
// ✅ Array destructuring with defaults
let a, b;
[a = 10, b = 20] = [1]; // a === 1, b === 20

// ✅ Object destructuring with defaults (missing properties)
let name, age;
{name: name = "Unknown", age: age = 0} = {}; // name === "Unknown", age === 0

// ✅ Mixed - some with values, some with defaults
let x, y, z;
[x = 1, y = 2, z = 3] = [100, 200]; // x === 100, y === 200, z === 3

// ✅ Object shorthand with defaults for missing properties
let prop;
{prop: prop = 42} = {}; // prop === 42
```

**Ready for Phase 4:** Default value implementation is complete. Next step is implementing rest elements (`...rest`) in array destructuring patterns.

---

## ✅ Inline Destructuring Declarations Implementation Summary

**Date Completed:** December 2024

**What Was Implemented:**
- ✅ **AST Extensions**: Added `ArrayDestructuringDeclaration` and `ObjectDestructuringDeclaration` nodes for inline declarations
- ✅ **Parser Integration**: Modified `parseLetStatement` and `parseConstStatement` to detect and parse destructuring patterns 
- ✅ **Type Checker Support**: Added comprehensive type checking for destructuring declarations with proper variable definition and type inference
- ✅ **Compiler Implementation**: Implemented bytecode generation that desugars to existing VM operations using `defineDestructuredVariable` helpers
- ✅ **Testing Suite**: Created 6 comprehensive test cases covering all inline destructuring scenarios

**Key Technical Achievements:**
- **Zero new VM opcodes** required - leverages existing `OpGetIndex`, `OpGetProp`, `OpSetGlobal`, `OpMove`, `OpJumpIfUndefined`
- **Efficient variable definition**: Properly handles both global and local variable creation in different scopes
- **Full TypeScript compatibility**: Complete type checking with proper error reporting for invalid patterns
- **Comprehensive syntax support**: Handles both `let`/`const` with arrays/objects and default values

**Supported Syntax:**
```typescript
// ✅ Array destructuring declarations
let [a, b] = [1, 2];
const [x, y] = [10, 20];

// ✅ Object destructuring declarations  
let {name, age} = person;
const {x, y} = point;

// ✅ Default values in declarations
let [a = 5, b = 10] = [1];
const {x: x = 100, y: y = 200} = {x: 50};

// ✅ Mixed scenarios
let [first, second = "default"] = ["hello"];
const {prop = 42} = {};
```

**Test Results:**
All 6 test cases pass successfully:
- ✅ `destructuring_declarations_basic.ts` - Basic array destructuring
- ✅ `destructuring_declarations_array_second.ts` - Array second element  
- ✅ `destructuring_declarations_object_basic.ts` - Basic object destructuring
- ✅ `destructuring_declarations_const_array.ts` - Const array destructuring
- ✅ `destructuring_declarations_const_object.ts` - Const object destructuring
- ✅ `destructuring_declarations_defaults.ts` - Destructuring with defaults

**Impact:** This implementation answers the user's question: "why can't we do it inline with declaration i.e. `let {x,y} = ...` or `const {x,y} = ...`" - **now we can!** The feature builds perfectly on the existing destructuring assignment foundation and provides the missing inline declaration capability.

**Ready for Phase 4:** With both destructuring assignments and inline declarations complete, the next logical step is implementing rest elements (`...rest`) in array destructuring patterns.

---

## ✅ Phase 4 Completion Summary - Rest Elements

**Date Completed:** December 2024

**What Was Implemented:**
- ✅ **AST Extensions**: Added `IsRest` field to `DestructuringElement` to mark rest elements (`...rest`)
- ✅ **Parser Integration**: Enhanced both assignment and declaration parsing to detect `...` patterns and validate placement (rest must be last)
- ✅ **Type Checker Support**: Rest elements receive `ArrayType` with proper element types - handles both regular arrays and precise tuple slicing
- ✅ **New VM Opcode**: Added `OpArraySlice` for efficient array slicing operations (`Rx Ry Rz: Rx = Ry.slice(Rz)`)
- ✅ **Compiler Implementation**: Desugars rest elements to specialized slice operations with register-efficient compilation
- ✅ **Testing Suite**: Created 6 comprehensive test cases covering all rest element scenarios

**Key Technical Achievements:**
- **Optimized VM opcode**: `OpArraySlice` provides efficient array slicing without method calls or external dependencies  
- **Robust type inference**: Rest elements get precise `ArrayType` with union types for remaining tuple elements
- **Complete syntax validation**: Parser enforces rest elements must be last and only one per pattern
- **Efficient compilation**: Uses specialized `compileArraySliceCall` helper for optimal bytecode generation

**Supported Syntax:**
```typescript
// ✅ Basic rest elements in assignments
let first = 0, rest = [];
[first, ...rest] = [1, 2, 3, 4, 5]; // rest === [2, 3, 4, 5]

// ✅ Rest elements in declarations  
let [head, ...tail] = ["a", "b", "c", "d"]; // tail === ["b", "c", "d"]
const [x, ...remaining] = [10, 20, 30]; // remaining === [20, 30]

// ✅ Rest-only destructuring
let [...everything] = [100, 200, 300]; // everything === [100, 200, 300]

// ✅ Empty rest arrays
let [a, b, ...empty] = [1, 2]; // empty === []
```

**VM Implementation Details:**
- **OpArraySlice opcode**: Efficient 3-register operation for array slicing from start index
- **Negative index support**: Handles JavaScript-style negative indexing and boundary clamping
- **Memory efficient**: Creates new arrays only for the sliced portion, not full copies

**Test Results:**
All 6 rest element tests pass successfully:
- ✅ `destructuring_rest_basic.ts` - Basic rest element extraction
- ✅ `destructuring_rest_length.ts` - Rest array length verification  
- ✅ `destructuring_rest_empty.ts` - Empty rest arrays
- ✅ `destructuring_rest_declaration.ts` - Rest in let declarations
- ✅ `destructuring_rest_const.ts` - Rest in const declarations
- ✅ `destructuring_rest_only.ts` - Rest-only patterns

**Performance Impact:**
- **Zero overhead** for non-rest destructuring (existing patterns unchanged)
- **Single opcode** for rest operations instead of method calls
- **Efficient memory usage** with precise array slicing

**Ready for Phase 5:** Rest elements complete the core destructuring feature set. Next step is implementing function parameter destructuring to enable destructuring in function signatures.
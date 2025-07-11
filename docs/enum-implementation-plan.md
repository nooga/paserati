# Enum Implementation Plan

This document outlines the comprehensive implementation plan for TypeScript enums in Paserati.

## Overview

Enums in TypeScript serve a dual purpose as both **types** and **runtime values**, similar to classes. They provide a way to define a set of named constants with strong type checking and runtime introspection capabilities.

## TypeScript Enum Rules & Behavior

### 1. Numeric Enums (Default)

```typescript
enum Direction {
    Up,      // 0
    Down,    // 1  
    Left,    // 2
    Right    // 3
}

// Compiles to:
// { 0: "Up", 1: "Down", 2: "Left", 3: "Right", Up: 0, Down: 1, Left: 2, Right: 3 }
```

**Key Features:**
- **Bidirectional mapping**: `Direction.Up` → `0` and `Direction[0]` → `"Up"`
- **Auto-incrementing**: Starts at 0, increments by 1
- **Custom values**: `enum E { A = 5, B }` → B becomes 6
- **Computed values**: `enum E { A = getValue(), B }` → B = A + 1

### 2. String Enums

```typescript
enum Status {
    Loading = "loading",
    Success = "success", 
    Error = "error"
}

// Compiles to:
// { Loading: "loading", Success: "success", Error: "error" }
```

**Key Features:**
- **No reverse mapping** (only forward mapping)
- **All members must be initialized** explicitly
- **More readable runtime values**

### 3. Heterogeneous Enums (Mixed)

```typescript
enum Mixed {
    A,              // 0
    B = "hello",    // "hello"
    C = 5,          // 5
    D               // 6
}
```

**Features:**
- **Allowed but discouraged** in TypeScript
- **Complex runtime behavior**

### 4. Const Enums

```typescript
const enum Colors {
    Red,    // 0
    Green,  // 1
    Blue    // 2
}

// Usage: Colors.Red is inlined to 0 at compile time
// No runtime object is generated
```

## Type System Integration

### Enum Types vs Member Literal Types

```typescript
enum Color { Red, Green, Blue }

// Enum type (union of all members)
let color: Color = Color.Red;           // ✅ Any Color member

// Enum member literal type (specific member)
let red: Color.Red = Color.Red;         // ✅ Only Color.Red
let red2: Color.Red = Color.Green;      // ❌ Error!
```

### Assignment Rules

```typescript
enum E { A, B }

// TypeScript strict mode behavior:
let x: E = 0;        // ❌ Error - raw numbers not assignable
let y: E = E.A;      // ✅ OK - enum member
let z: E = E["A"];   // ✅ OK - computed access
```

### Type Operations

```typescript
enum E { A, B, C }

type Keys = keyof E;              // "A" | "B" | "C" (not reverse mapping keys)
type Values = E[keyof E];         // E.A | E.B | E.C
type AType = E.A;                 // Literal type E.A
```

## Implementation Architecture

### Phase 1: Core Enum Support

#### 1.1 Lexer Changes
- Add `ENUM` token type
- Add `"enum"` keyword mapping

#### 1.2 Parser Changes
```typescript
// AST Node Structure
interface EnumDeclaration {
    Token: lexer.Token;           // 'enum' keyword
    Name: Identifier;             // Enum name
    Members: EnumMember[];        // Enum members
    IsConst: boolean;             // const enum flag
}

interface EnumMember {
    Token: lexer.Token;           // Member name token
    Name: Identifier;             // Member name
    Value?: Expression;           // Optional initializer
}
```

#### 1.3 Type System Changes
```typescript
// New type representations
interface EnumType {
    Name: string;
    Members: map[string]EnumMemberType;
    IsConst: boolean;
}

interface EnumMemberType {
    Name: string;
    EnumName: string;
    Value: any;                   // Runtime value (number/string)
    LiteralType: Type;            // E.Member literal type
}
```

### Phase 2: Runtime Implementation

#### 2.1 Numeric Enum Object Generation
```javascript
// Target runtime structure for enum Direction { Up, Down, Left, Right }
var Direction = (function() {
    var obj = {};
    // Forward mapping
    obj.Up = 0;
    obj.Down = 1;
    obj.Left = 2;
    obj.Right = 3;
    // Reverse mapping
    obj[0] = "Up";
    obj[1] = "Down";
    obj[2] = "Left";
    obj[3] = "Right";
    return obj;
})();
```

#### 2.2 String Enum Object Generation
```javascript
// Target runtime structure for enum Status { Loading = "loading", Success = "success" }
var Status = {
    Loading: "loading",
    Success: "success"
    // No reverse mapping for string enums
};
```

### Phase 3: Module Integration

#### 3.1 Export/Import Behavior
```typescript
// module.ts
export enum Color { Red, Green, Blue }

// consumer.ts
import { Color } from "./module";     // Imports both type and value contexts
```

#### 3.2 Dual Context Handling
- Enums exist in both **type** and **value** namespaces
- Similar to classes, need special handling in symbol resolution
- Module system must export/import both contexts

### Phase 4: Advanced Features

#### 4.1 Const Enum Inlining
```typescript
const enum E { A, B }
let x = E.A;  // Should inline to: let x = 0;
```

#### 4.2 Computed Members
```typescript
enum E {
    A = getValue(),
    B = A + 1
}
```

## Implementation Steps

### Step 1: Basic Numeric Enums
1. **Lexer**: Add ENUM token
2. **Parser**: EnumDeclaration and EnumMember AST nodes
3. **Type Checker**: Basic enum type creation and member resolution
4. **Compiler**: Simple numeric enum object generation
5. **Tests**: Basic numeric enum functionality

### Step 2: String Enums
1. **Parser**: String literal initializers
2. **Type Checker**: String enum member types
3. **Compiler**: String enum object generation (no reverse mapping)
4. **Tests**: String enum functionality

### Step 3: Type System Integration
1. **Type Checker**: Enum member literal types
2. **Type Checker**: Assignment rules and validation
3. **Type Checker**: keyof and indexed access support
4. **Tests**: Type checking and assignment behavior

### Step 4: Module System Integration
1. **Symbol Table**: Dual context enum symbols
2. **Module System**: Export/import handling
3. **Tests**: Import/export scenarios

### Step 5: Advanced Features
1. **Const enums**: Compile-time inlining
2. **Computed members**: Expression evaluation
3. **Heterogeneous enums**: Mixed number/string support

## Performance Considerations

### Runtime Efficiency
- **Single object creation**: Avoid temporary objects during enum generation
- **Property access**: Standard object property lookup (O(1))
- **Memory usage**: Efficient reverse mapping for numeric enums only

### Compile-time Efficiency
- **Type checking**: Fast enum member lookup with hash tables
- **Const enum inlining**: Compile-time value resolution
- **Import resolution**: Efficient dual-context symbol handling

## Testing Strategy

### Isolated Feature Tests
1. **Basic numeric enums**: Auto-increment, custom values
2. **String enums**: Explicit values, no reverse mapping
3. **Mixed enums**: Heterogeneous value types
4. **Type checking**: Assignment rules, literal types
5. **Module integration**: Import/export behavior
6. **Error cases**: Invalid syntax, type mismatches

### Integration Tests
1. **With classes**: Enum types as class members
2. **With generics**: Enum types as type arguments
3. **With utility types**: keyof, indexed access
4. **With control flow**: Enum value comparisons

## Error Handling

### Compile-time Errors
- **Duplicate member names**: `enum E { A, A }`
- **Invalid initializers**: `enum E { A = "hello", B }` (string followed by auto-increment)
- **Type mismatches**: Assigning wrong types to enum variables
- **Const enum violations**: Runtime access to const enum objects

### Runtime Considerations
- **Enum objects are frozen**: Prevent runtime modification
- **Reverse mapping consistency**: Ensure bidirectional mapping integrity
- **Module loading**: Proper enum object initialization

## Future Extensions

### TypeScript 5.0+ Features
- **Template literal enum patterns**
- **Enum member names as types**
- **Enhanced const enum optimizations**

### Performance Optimizations
- **Enum value inlining**: Optimize frequent enum comparisons
- **Dead code elimination**: Remove unused enum members
- **Bundle size optimization**: Minimize enum runtime footprint

---

This implementation plan provides a comprehensive roadmap for adding full TypeScript enum support to Paserati, ensuring compatibility, performance, and maintainability.
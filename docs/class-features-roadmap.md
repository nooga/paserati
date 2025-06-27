# TypeScript Class Features Implementation Roadmap

## Overview

This document outlines the step-by-step implementation plan for TypeScript class features in Paserati. 

**🎉 MAJOR MILESTONE ACHIEVED**: Core TypeScript class features are now **production-ready**!

### 📊 Implementation Progress
- **✅ Phase 1 (Type System Integration)**: 100% Complete
- **🔄 Phase 2 (Access Modifiers)**: 33% Complete (1/3 features)
- **✅ Phase 3 (Optional Features)**: 100% Complete
- **✅ Bonus Features**: 100% Complete

### 🚀 What Works Now
All fundamental TypeScript class features are **fully implemented and working**:
- Property type annotations, initializers, and optional properties
- Method type annotations (parameters and return types)
- Static members (properties and methods)
- Constructor optional parameter handling
- Class names as type annotations
- TypeScript-style declaration merging

## Current Status

### ✅ Working Features
- Basic class declarations: `class Name {}`
- Property declarations without types: `prop;`, `prop = value;`
- **✅ Property declarations with types: `name: string;`** (only with built-in type identifiers available in scope)
- **✅ Property initializers: `score: number = 100;`** (type annotations work with available types)
- **✅ Optional properties: `email?: string;`**
- Constructor methods: `constructor() {}`
- **✅ Constructor with typed parameters: `constructor(name: string, age?: number)`**
- **✅ Constructor optional parameter arity handling**
- Instance methods: `method() {}`
- **✅ Method return type annotations: `method(): string`**
- **✅ Method parameter type annotations: `method(param: string, optional?: number)`**
- **✅ Static properties: `static count: number = 0;`**
- **✅ Static methods: `static getCount(): number`**
- **✅ Class names as type annotations: `createDefault(): ClassName`**
- **✅ Readonly properties: `readonly id: number;`**
- **✅ Static readonly properties: `static readonly version = "1.0";`**
- **✅ Combined modifiers: Both `static readonly` and `readonly static` orders work**
- **✅ Readonly assignment enforcement at compile-time**
- **✅ Utility type: `Readonly<T>` (basic support)**
- Class instantiation: `new Class()`
- Property access and method calls

### ❌ Missing Features (Parsing/Implementation Gaps)
- Access modifiers are parsed but not enforced: `private`, `public`, `protected`
- **✅ Readonly modifier: `readonly prop: type`** - COMPLETED
- Class inheritance: `extends` keyword
- Super calls: `super()` and `super.method()`

## Implementation Phases

### Phase 1: Type System Integration (High Priority) ✅ **COMPLETED**

#### 1.1 Property Type Annotations ✅ **COMPLETED**
**Goal**: Support `name: string;` syntax in class bodies

~~**Current Issue**:~~
```typescript
class User {
    name: string;  // ✅ Now works perfectly!
}
```

**✅ Completed Implementation**:
- **File**: `pkg/parser/parse_class.go`
- **Method**: `parseProperty()`
- **Changes**: ✅ All implemented
  1. ✅ After parsing property identifier, check for `:` token
  2. ✅ Parse type annotation using existing type parsing logic
  3. ✅ Store type in `PropertyDefinition.TypeAnnotation` field
  4. ✅ Updated AST dump to show property types

**✅ Test Files**: `tests/scripts/class_FIXME_type_annotations.ts` - **NOW PASSING**

#### 1.2 Method Return Type Annotations ✅ **COMPLETED**
**Goal**: Support `method(): returnType` syntax

~~**Current Issue**:~~
```typescript
getName(): string { return this.name; }  // ✅ Now works perfectly!
```

**✅ Completed Implementation**:
- **File**: `pkg/parser/parse_class.go`
- **Method**: Method parsing in `parseClassBody()`
- **Changes**: ✅ All implemented
  1. ✅ After parsing method parameters `()`, check for `:` token
  2. ✅ Parse return type annotation
  3. ✅ Store in `FunctionLiteral.ReturnTypeAnnotation`
  4. ✅ Enhanced AST dump to show return type annotations

#### 1.3 Method Parameter Type Annotations ✅ **COMPLETED**
**Goal**: Support `method(param: type)` syntax

~~**Current Issue**:~~
```typescript
setName(name: string) { }  // ✅ Already worked via unified parameter handling!
```

**✅ Discovery**: Was already working via unified parameter parsing across all function types (methods, functions, arrows)

#### 1.4 Constructor Parameter Types ✅ **COMPLETED**
**Goal**: Support typed constructor parameters

**✅ Completed**: Uses same logic as 1.3 since constructors use standard function parameter parsing
**✅ Bonus**: Fixed constructor optional parameter arity handling to match function behavior

### Phase 2: Access Modifiers (Medium Priority) 🔄 **PARTIALLY COMPLETED**

#### 2.1 Basic Access Modifiers ❌ **NOT IMPLEMENTED**
**Goal**: Support `public`, `private`, `protected` keywords

**Current Issue**:
```typescript
private name: string;  // ❌ Still parsed as property named "private"
```

**Implementation Plan**:
- **File**: `pkg/parser/parse_class.go`
- **Method**: `parseClassBody()` and `parseProperty()`
- **Changes**:
  1. Add lexer tokens for `PUBLIC`, `PRIVATE`, `PROTECTED` if not exist
  2. Check for access modifier tokens before property/method parsing
  3. Add access modifier fields to `PropertyDefinition` and `MethodDefinition`
  4. Update parser to consume modifier tokens

#### 2.2 Static Members ✅ **COMPLETED**
**Goal**: Support `static` keyword for properties and methods

**✅ Completed Implementation**:
- **File**: `pkg/parser/parse_class.go` and `pkg/compiler/compile_class.go` and `pkg/checker/class.go`
- **Changes**: ✅ All implemented
  1. ✅ `STATIC` token already recognized by parser
  2. ✅ Parser sets `IsStatic` field in AST nodes
  3. ✅ Compiler attaches static members to constructor function
  4. ✅ Type checker includes static members in constructor type
  5. ✅ Runtime execution works perfectly

**✅ Test Files**: `tests/scripts/class_static_members.ts` - **NOW PASSING**

#### 2.3 Readonly Properties ❌ **NOT IMPLEMENTED**
**Goal**: Support `readonly` modifier

**Current Issue**:
```typescript
readonly id: number = 42;  // ❌ Parsed as property named "readonly"
```

**Implementation Plan**:
- **File**: `pkg/parser/parse_class.go`
- **Changes**:
  1. Add `READONLY` token
  2. Parse `readonly` modifier for properties
  3. Add `IsReadonly` field to `PropertyDefinition`

### Phase 3: Optional Features (Medium Priority) ✅ **COMPLETED**

#### 3.1 Optional Properties and Parameters ✅ **COMPLETED**
**Goal**: Support `?` syntax for optional members

**✅ Completed Implementation**:
- **File**: `pkg/parser/parse_class.go` and parameter parsing
- **Changes**: ✅ All implemented
  1. ✅ Check for `?` token after property/parameter names
  2. ✅ Set `Optional` field in respective AST nodes
  3. ✅ TypeScript-style declaration merging implemented
  4. ✅ Constructor optional parameter arity handling fixed

**✅ Test Files**: `tests/scripts/class_FIXME_optional_properties.ts` - **NOW PASSING**

### Phase 4: Inheritance (Lower Priority)

#### 4.1 Class Inheritance
**Goal**: Support `extends` keyword

**Implementation Plan**:
- **File**: `pkg/parser/parse_class.go`
- **Method**: `parseClassDeclaration()` and `parseClassExpression()`
- **Changes**:
  1. After class name, check for `EXTENDS` token
  2. Parse superclass identifier
  3. Update type checker for inheritance

#### 4.2 Super Calls
**Goal**: Support `super()` and `super.method()` calls

**Implementation Plan**:
- **File**: `pkg/parser/parser.go` (expression parsing)
- **Changes**:
  1. Add `SUPER` token support
  2. Create `SuperExpression` AST node
  3. Handle `super()` calls and `super.property` access

### Phase 5: Advanced Features (Future)

#### 5.1 Getters and Setters
**Goal**: Support `get`/`set` method syntax

#### 5.2 Abstract Classes
**Goal**: Support `abstract` keyword and abstract methods

#### 5.3 Generic Classes
**Goal**: Support `class Container<T>` syntax

#### 5.4 Interface Implementation
**Goal**: Support `implements` keyword

## Implementation Guidelines

### Testing Strategy
1. **Use existing FIXME test files** to validate each feature
2. **Run AST dump** (`-ast` flag) to verify correct parsing structure
3. **Check type checker integration** after parser changes
4. **Ensure backwards compatibility** with existing working features

### Code Organization
1. **Lexer changes**: Add new tokens in `pkg/lexer/lexer.go`
2. **Parser changes**: Primarily in `pkg/parser/parse_class.go`
3. **AST updates**: Extend existing nodes or add new ones in `pkg/parser/ast.go`
4. **Type checker**: Update class handling in `pkg/checker/class.go`

### Validation Process
1. Build: `go build -o paserati cmd/paserati/main.go`
2. Test specific feature: `./paserati -ast tests/scripts/class_FIXME_*.ts`
3. Run test suite: `go test ./tests -run TestScripts`
4. Verify AST structure shows correct parsing

## Success Criteria

### Phase 1 Complete ✅ **COMPLETED**
- [x] Property type annotations parse correctly
- [x] Method return types parse correctly  
- [x] Method parameter types parse correctly
- [x] Constructor parameter types parse correctly
- [x] All `class_FIXME_type_annotations.ts` tests pass

### Phase 2 Complete 🔄 **PARTIALLY COMPLETED**
- [ ] Access modifiers (`public`, `private`, `protected`) parse correctly
- [x] Static members parse and work correctly
- [ ] Readonly properties are recognized
- [ ] All `class_FIXME_access_modifiers.ts` tests pass

### Phase 3 Complete ✅ **COMPLETED**
- [x] Optional properties and parameters work correctly
- [x] Constructor optional parameter arity handling fixed
- [x] All `class_FIXME_optional_properties.ts` tests pass

### Bonus Features Complete ✅ **COMPLETED**
- [x] Field initializers execute during object construction
- [x] Class names work as type annotations (e.g., `method(): ClassName`)
- [x] TypeScript-style declaration merging implemented

### Final Success
- [ ] All comprehensive class test files pass
- [ ] Type checker properly handles all class features
- [ ] No regressions in existing functionality
- [ ] AST dump shows proper structure for all features

## File Reference

### Test Files Created
- `tests/scripts/class_FIXME_type_annotations.ts` - Type annotation features
- `tests/scripts/class_FIXME_access_modifiers.ts` - Access modifiers  
- `tests/scripts/class_FIXME_static_members.ts` - Static properties/methods
- `tests/scripts/class_FIXME_inheritance.ts` - Class inheritance
- `tests/scripts/class_FIXME_optional_properties.ts` - Optional features
- Plus comprehensive advanced feature tests

### Key Implementation Files
- `pkg/parser/parse_class.go` - Main class parsing logic
- `pkg/parser/parser.go` - General parsing utilities
- `pkg/parser/ast.go` - AST node definitions
- `pkg/lexer/lexer.go` - Token definitions
- `pkg/checker/class.go` - Type checking for classes

This roadmap provides a clear, implementable path to full TypeScript class support in Paserati.

## Recent Achievements

### Readonly Implementation (Completed)
- **✅ Added `readonly` keyword to lexer** (`pkg/lexer/lexer.go`)
- **✅ Added `Readonly` field to PropertyDefinition AST** (`pkg/parser/ast.go`)
- **✅ Updated class parser to handle readonly modifier** (`pkg/parser/parse_class.go`)
- **✅ Created ReadonlyType wrapper type** (`pkg/types/readonly.go`)
- **✅ Implemented Readonly<T> utility type** (`pkg/types/generic.go`)
- **✅ Added readonly assignment checking** (`pkg/checker/assignment.go`)
- **✅ Updated type assignability rules for readonly** (`pkg/types/assignable.go`)
- **✅ Registered Readonly<T> as global utility type** (`pkg/builtins/utility_types_init.go`)
- **✅ Fixed modifier parsing to support both `static readonly` and `readonly static` orders** (`pkg/parser/parse_class.go`)

The readonly implementation follows TypeScript semantics:
- Properties marked as `readonly` cannot be reassigned after initialization
- `readonly T` is assignable to `T` (covariance)
- `T` is NOT assignable to `readonly T` (prevents mutation)
- `Readonly<T>` is recognized as a valid generic type (like `Array<T>`)
- Note: Full mapped type semantics for `Readonly<T>` are not yet implemented
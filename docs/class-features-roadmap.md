# TypeScript Class Features Implementation Roadmap

## Overview

This document outlines the step-by-step implementation plan for TypeScript class features in Paserati. Based on comprehensive testing with the AST dump utility, we've identified exactly what's working and what needs to be implemented.

## Current Status

### ✅ Working Features
- Basic class declarations: `class Name {}`
- Property declarations without types: `prop;`, `prop = value;`
- Constructor methods: `constructor() {}`
- Instance methods: `method() {}`
- Class instantiation: `new Class()`
- Property access and method calls
- Method parameters without types: `method(param1, param2)`

### ❌ Missing Features (Parsing Errors)
All advanced TypeScript class syntax fails with parsing errors, specifically:
- Type annotations cause "expected identifier in class body" errors
- Access modifiers are parsed as property names
- TypeScript-specific keywords are not recognized in class context

## Implementation Phases

### Phase 1: Type System Integration (High Priority)

#### 1.1 Property Type Annotations
**Goal**: Support `name: string;` syntax in class bodies

**Current Issue**:
```typescript
class User {
    name: string;  // ❌ Parser error: "expected identifier in class body"
}
```

**Implementation Plan**:
- **File**: `pkg/parser/parse_class.go`
- **Method**: `parseProperty()`
- **Changes**:
  1. After parsing property identifier, check for `:` token
  2. If found, parse type annotation using existing type parsing logic
  3. Store type in `PropertyDefinition.TypeAnnotation` field (add if needed)
  4. Update AST dump to show property types

**Test Files**: `tests/scripts/class_FIXME_type_annotations.ts`

#### 1.2 Method Return Type Annotations
**Goal**: Support `method(): returnType` syntax

**Current Issue**:
```typescript
getName(): string { return this.name; }  // ❌ Parser error
```

**Implementation Plan**:
- **File**: `pkg/parser/parse_class.go`
- **Method**: Method parsing in `parseClassBody()`
- **Changes**:
  1. After parsing method parameters `()`, check for `:` token
  2. Parse return type annotation
  3. Store in `FunctionLiteral.ReturnTypeAnnotation`
  4. Ensure compatibility with existing function parsing

#### 1.3 Method Parameter Type Annotations
**Goal**: Support `method(param: type)` syntax

**Current Issue**:
```typescript
setName(name: string) { }  // ❌ Parser error
```

**Implementation Plan**:
- **File**: `pkg/parser/parser.go` (function parameter parsing)
- **Method**: `parseFunctionParameters()`
- **Changes**:
  1. Extend parameter parsing to handle `: type` after parameter name
  2. Store type in `Parameter.TypeAnnotation`
  3. Ensure this works for both regular functions and class methods

#### 1.4 Constructor Parameter Types
**Goal**: Support typed constructor parameters

**Implementation Plan**:
- Uses same logic as 1.3 since constructors use standard function parameter parsing
- Test specifically with class constructors

### Phase 2: Access Modifiers (Medium Priority)

#### 2.1 Basic Access Modifiers
**Goal**: Support `public`, `private`, `protected` keywords

**Current Issue**:
```typescript
private name: string;  // ❌ Parsed as property named "private"
```

**Implementation Plan**:
- **File**: `pkg/parser/parse_class.go`
- **Method**: `parseClassBody()` and `parseProperty()`
- **Changes**:
  1. Add lexer tokens for `PUBLIC`, `PRIVATE`, `PROTECTED` if not exist
  2. Check for access modifier tokens before property/method parsing
  3. Add access modifier fields to `PropertyDefinition` and `MethodDefinition`
  4. Update parser to consume modifier tokens

#### 2.2 Static Members
**Goal**: Support `static` keyword for properties and methods

**Implementation Plan**:
- **File**: `pkg/parser/parse_class.go`
- **Changes**:
  1. Add `STATIC` token recognition
  2. Parse `static` before property/method declarations
  3. Set `IsStatic` field in AST nodes
  4. Update type checker to handle static members

#### 2.3 Readonly Properties
**Goal**: Support `readonly` modifier

**Implementation Plan**:
- **File**: `pkg/parser/parse_class.go`
- **Changes**:
  1. Add `READONLY` token
  2. Parse `readonly` modifier for properties
  3. Add `IsReadonly` field to `PropertyDefinition`

### Phase 3: Optional Features (Medium Priority)

#### 3.1 Optional Properties and Parameters
**Goal**: Support `?` syntax for optional members

**Implementation Plan**:
- **File**: `pkg/parser/parse_class.go` and parameter parsing
- **Changes**:
  1. Check for `?` token after property/parameter names
  2. Set `Optional` field in respective AST nodes

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

### Phase 1 Complete
- [ ] Property type annotations parse correctly
- [ ] Method return types parse correctly  
- [ ] Method parameter types parse correctly
- [ ] Constructor parameter types parse correctly
- [ ] All `class_FIXME_type_annotations.ts` tests pass

### Phase 2 Complete
- [ ] Access modifiers (`public`, `private`, `protected`) parse correctly
- [ ] Static members parse and work correctly
- [ ] Readonly properties are recognized
- [ ] All `class_FIXME_access_modifiers.ts` tests pass

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
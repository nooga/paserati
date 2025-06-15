# Generics Implementation Plan

## Overview

This document outlines the implementation strategy for adding generic types to Paserati. The approach is pragmatic and incremental, starting with hardcoded support for built-in generic types before expanding to user-defined generics.

## Design Philosophy

Generics in Paserati are conceptually "type-level lambdas" - functions that take type arguments and produce concrete types. However, for practical implementation, we'll start with simpler approaches and gradually build toward this ideal.

## Implementation Phases

### Phase 1: Built-in Generic Type References
Support for `Array<T>`, `Promise<T>`, and other built-in generic types with explicit type arguments.

### Phase 2: Generic Functions with Explicit Type Arguments
Basic generic function declarations and calls with manual type argument specification.

### Phase 3: Type Inference
Automatic inference of type arguments in common cases.

### Phase 4: User-defined Generic Types
Generic interfaces, type aliases, and classes.

## Phase 1: Built-in Generic Type References (Detailed Implementation)

### 1.1 Lexer Changes

Add tokens for generic syntax:

```go
// pkg/lexer/lexer.go
// Add to TokenType enum:
LESS        // <
GREATER     // >
```

### 1.2 AST Additions

```go
// pkg/parser/ast.go

// GenericTypeRef represents Array<string>, Promise<number>, etc.
type GenericTypeRef struct {
    BaseExpression
    Name          *Identifier    // The generic type name (e.g., "Array")
    TypeArguments []Expression   // The type arguments (e.g., [string])
}

func (g *GenericTypeRef) String() string {
    args := make([]string, len(g.TypeArguments))
    for i, arg := range g.TypeArguments {
        args[i] = arg.String()
    }
    return fmt.Sprintf("%s<%s>", g.Name.Value, strings.Join(args, ", "))
}
```

### 1.3 Parser Modifications

The main challenge is disambiguating `<` as less-than vs. generic start. Strategy:

```go
// pkg/parser/parser.go

// In parseTypeAnnotation or similar:
func (p *Parser) parseTypeReference() Expression {
    name := p.parseIdentifier()
    
    // Check if this could be a generic type reference
    if p.match(lexer.LESS) {
        // Try to parse as generic, backtrack if it fails
        checkpoint := p.current
        typeArgs, ok := p.tryParseTypeArguments()
        if !ok {
            p.current = checkpoint
            return name
        }
        
        return &GenericTypeRef{
            BaseExpression: BaseExpression{Token: name.Token},
            Name:          name,
            TypeArguments: typeArgs,
        }
    }
    
    return name
}

func (p *Parser) tryParseTypeArguments() ([]Expression, bool) {
    // Already consumed '<'
    var args []Expression
    
    // Empty type arguments not allowed
    if p.check(lexer.GREATER) {
        return nil, false
    }
    
    for {
        arg := p.parseTypeAnnotation()
        if arg == nil {
            return nil, false
        }
        args = append(args, arg)
        
        if !p.match(lexer.COMMA) {
            break
        }
    }
    
    if !p.match(lexer.GREATER) {
        return nil, false
    }
    
    return args, true
}
```

### 1.4 Type System Additions

```go
// pkg/types/generic.go

// GenericInstanceType represents an instantiated generic type
// For Phase 1, this is temporary - we'll immediately resolve to concrete types
type GenericInstanceType struct {
    BaseName      string
    TypeArguments []Type
}

func (g *GenericInstanceType) String() string {
    args := make([]string, len(g.TypeArguments))
    for i, arg := range g.TypeArguments {
        args[i] = arg.String()
    }
    return fmt.Sprintf("%s<%s>", g.BaseName, strings.Join(args, ", "))
}

func (g *GenericInstanceType) Equals(other Type) bool {
    if o, ok := other.(*GenericInstanceType); ok {
        if g.BaseName != o.BaseName || len(g.TypeArguments) != len(o.TypeArguments) {
            return false
        }
        for i, arg := range g.TypeArguments {
            if !arg.Equals(o.TypeArguments[i]) {
                return false
            }
        }
        return true
    }
    return false
}

func (g *GenericInstanceType) typeNode() {}
```

### 1.5 Type Checker Modifications

```go
// pkg/checker/type_utils.go or checker.go

// Modify resolveTypeAnnotation to handle GenericTypeRef
func (c *Checker) resolveTypeAnnotation(expr parser.Expression) types.Type {
    switch e := expr.(type) {
    case *parser.GenericTypeRef:
        return c.resolveGenericType(e)
    // ... existing cases
    }
}

// resolveGenericType handles built-in generic types
func (c *Checker) resolveGenericType(ref *parser.GenericTypeRef) types.Type {
    // Resolve type arguments first
    typeArgs := make([]types.Type, len(ref.TypeArguments))
    for i, arg := range ref.TypeArguments {
        typeArgs[i] = c.resolveTypeAnnotation(arg)
        if typeArgs[i] == nil {
            c.addError(arg.GetToken(), "Invalid type argument")
            return types.AnyType
        }
    }
    
    // Handle built-in generics
    switch ref.Name.Value {
    case "Array":
        if len(typeArgs) != 1 {
            c.addError(ref.Token, "Array requires exactly one type argument")
            return types.AnyType
        }
        return &types.ArrayType{ElementType: typeArgs[0]}
        
    case "Promise":
        if len(typeArgs) != 1 {
            c.addError(ref.Token, "Promise requires exactly one type argument")
            return types.AnyType
        }
        // Return a Promise type (might need to create PromiseType or use ObjectType)
        return c.createPromiseType(typeArgs[0])
        
    case "Record":
        if len(typeArgs) != 2 {
            c.addError(ref.Token, "Record requires exactly two type arguments")
            return types.AnyType
        }
        // Validate first arg is string or string literal union
        if !isValidRecordKey(typeArgs[0]) {
            c.addError(ref.TypeArguments[0].GetToken(), "Record key must be string, number, or symbol")
            return types.AnyType
        }
        return c.createRecordType(typeArgs[0], typeArgs[1])
        
    default:
        c.addError(ref.Token, fmt.Sprintf("Unknown generic type '%s'", ref.Name.Value))
        return types.AnyType
    }
}

// Helper to create Promise type
func (c *Checker) createPromiseType(valueType types.Type) types.Type {
    // Create ObjectType with Promise structure
    promiseType := types.NewObjectType()
    
    // Add 'then' method
    thenFunc := types.NewFunctionType(
        []types.Type{
            // onfulfilled: (value: T) => any
            types.NewFunctionType([]types.Type{valueType}, types.AnyType),
            // onrejected?: (reason: any) => any (optional)
            types.NewUnionType(
                types.UndefinedType,
                types.NewFunctionType([]types.Type{types.AnyType}, types.AnyType),
            ),
        },
        types.AnyType, // Returns Promise<any> for now
    )
    promiseType.WithProperty("then", thenFunc)
    
    // Add 'catch' method
    catchFunc := types.NewFunctionType(
        []types.Type{
            types.NewFunctionType([]types.Type{types.AnyType}, types.AnyType),
        },
        types.AnyType, // Returns Promise<any> for now
    )
    promiseType.WithProperty("catch", catchFunc)
    
    return promiseType
}
```

### 1.6 Testing

Create test files to verify Phase 1 functionality:

```typescript
// tests/scripts/generics_array_basic.ts
let arr1: Array<string> = ["hello", "world"];
let arr2: Array<number> = [1, 2, 3];
let arr3: Array<boolean> = [true, false];

// expect: undefined

// tests/scripts/generics_array_errors.ts
let arr1: Array<string> = [1, 2, 3];  // Type error
// expect_compile_error: Type 'number' is not assignable to type 'string'

// tests/scripts/generics_nested.ts
let matrix: Array<Array<number>> = [[1, 2], [3, 4]];
// expect: undefined

// tests/scripts/generics_promise.ts
let p: Promise<string>;
// expect: undefined
```

### 1.7 No VM Changes Required

Since we're using type erasure, `Array<string>` and `Array<number>` are both just arrays at runtime. No VM modifications needed for Phase 1.

## Phase 2: Generic Functions (Overview)

### Key Components:

1. **AST Changes**: Add type parameter list to FunctionExpression
2. **Type System**: Extend FunctionType to include type parameters
3. **Type Checker**: 
   - Create new environment scope with type parameters
   - Validate type arguments at call sites
   - Substitute type parameters in function body
4. **Parser**: Parse `function identity<T>(x: T): T` syntax

### Example Implementation Sketch:

```go
// AST addition
type TypeParameter struct {
    BaseExpression
    Name       *Identifier
    Constraint Expression  // Optional, e.g., "extends string"
}

// In FunctionExpression
type FunctionExpression struct {
    // ... existing fields ...
    TypeParameters []*TypeParameter  // NEW
}

// Type checker handling
func (c *Checker) visitFunctionExpression(expr *parser.FunctionExpression) types.Type {
    // Create new environment
    funcEnv := NewEnclosedEnvironment(c.env)
    
    // Add type parameters to environment
    for _, tp := range expr.TypeParameters {
        // Create TypeParameterType and add to funcEnv
    }
    
    // Continue with normal function checking in new environment
}
```

## Phase 3: Type Inference (Overview)

Basic inference algorithm:
1. Collect constraints from argument types
2. Unify constraints to solve for type parameters
3. Substitute solved types back into function signature

## Implementation Milestones

### Milestone 1: Basic Array<T> Support (Phase 1.1-1.4) ✅ COMPLETED
- [x] Add lexer tokens (LT/GT already existed)
- [x] Add GenericTypeRef AST node
- [x] Implement parser changes (simplified without full backtracking)
- [x] Add type checker support for Array<T>
- [x] Verify Array<T> unifies with T[] syntax
- [x] Basic test suite with error cases
- [x] Fix array literal type checking with contextual types

### Milestone 2: Core Generic Type System ✅ COMPLETED
- [x] Design and implement real generic type system (not hardcoded)
- [x] Add TypeParameter and TypeParameterType to type system
- [x] Implement InstantiatedType and GenericType
- [x] Create type substitution mechanism
- [x] Add comprehensive tests for generic types
- [x] Implement built-in generics (ArrayGeneric, PromiseGeneric)

### Milestone 3: Generic Functions (Phase 2) ✅ COMPLETED
- [x] Add TypeParameter AST node for parsing
- [x] Extend FunctionLiteral and ArrowFunctionLiteral AST for type params
- [x] Parse function type parameters (function<T>, <T>() => )
- [x] Parse arrow functions with type parameters
- [x] Support constraint syntax (T extends string)
- [x] Manual testing confirms parsing works
- [x] Extend Environment to track type parameters
- [x] Type check generic function bodies
- [x] Unified function checking architecture (FunctionCheckContext)
- [x] Type parameter consistency between hoisting and body checking
- [x] Enhanced IsAssignable for TypeParameterType equality
- [x] Error reporting for type parameter mismatches
- [x] Integration with existing checker phases
- [ ] Support default type parameters (T = any)
- [ ] Test edge cases

### Milestone 4: Type Inference (Phase 3) ✅ COMPLETED
- [x] Implement basic constraint collection from function arguments
- [x] Add unification algorithm for type parameter solving
- [x] Handle common inference patterns (identity, array methods)
- [x] Support generic function calls without explicit type arguments
- [x] Error reporting for inference failures
- [x] Constraint-based type inference with confidence scoring
- [x] Type substitution for function signature instantiation
- [x] Multiple type parameter inference support
- [x] Integration with call expression checking
- [x] Comprehensive testing of inference scenarios

### Milestone 5: Extended Built-ins (Phase 1.5-1.6) 📋 TODO
- [x] Add Promise<T> support (basic structure)
- [ ] Add Record<K, V> support (needs Record type first)
- [ ] Add Map<K, V> and Set<T> support (needs Map/Set types first)
- [ ] Comprehensive test suite for all built-ins

### Milestone 6: Advanced Generic Features 📋 FUTURE
- [ ] Support default type parameters (T = any)
- [ ] Generic function overloads
- [ ] User-defined generic types (interfaces, type aliases)
- [ ] Conditional types (T extends U ? A : B)
- [ ] Mapped types (keyof, in operator)
- [ ] Variance annotations (in T, out T)

## Success Criteria

1. Can parse and type-check `Array<string>`, `Promise<number>`, etc.
2. Proper error messages for type mismatches
3. No runtime overhead (type erasure)
4. Clean integration with existing type system
5. All existing tests continue to pass

## Open Questions

1. Should we support partial type argument inference in Phase 3?
2. How do we handle variance for generic types?
3. Should conditional types be part of Phase 4 or a separate phase?
4. How do we handle generic constraints beyond simple `extends`?

## References

- TypeScript's generic implementation
- V8's hidden class optimization (for future optimization ideas)
- Hindley-Milner type inference (for Phase 3)
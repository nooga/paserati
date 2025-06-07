# Unified Type System Refactoring Plan

Based on TypeScript's internal architecture research, we need to refactor Paserati's type system to a truly unified approach where:

1. **Eliminate FunctionType entirely** - everything is ObjectType with call signatures (like TypeScript)
2. **Add helper methods** on types for cleaner type checking (IsCallable, IsPrimitive, etc.)
3. **Move type operations** from checker to types package (isAssignable, widening, etc.)
4. **Inheritance works through base types** like TypeScript's prototype chain modeling
5. **Simplified architecture** following TypeScript's proven unified design

## Current Architecture Problems

1. **Multiple separate types create complexity**:

   - `FunctionType` + `ConstructorType` + `ObjectType` + `CallableType` + `OverloadedFunctionType`
   - Complex cross-type compatibility logic (600+ lines in `type_utils.go`)
   - Artificial boundaries between "functions" and "objects with call signatures"

2. **Type operations in wrong package**:

   - `isAssignable()` is in `checker` but it's pure type logic
   - `deeplyWidenType()` should be in `types`
   - Type checking mixed with type operations

3. **Missing inheritance modeling**:

   - No base types mechanism for class inheritance
   - No interface extension support
   - No proper prototype chain modeling

4. **No helpful type methods**:
   - Complex type switches everywhere in checker
   - No `IsCallable()`, `IsPrimitive()`, etc. helpers

## Target Architecture (Pure TypeScript Style)

```go
// ONLY unified ObjectType - represents everything (functions, objects, classes, interfaces)
type ObjectType struct {
    Properties          map[string]Type      // Properties & methods
    OptionalProperties  map[string]bool      // Optional properties
    CallSignatures      []*Signature         // Function calls: obj(args)
    ConstructSignatures []*Signature         // Constructor calls: new obj(args)
    BaseTypes           []Type               // Inheritance chain
}

// Signature represents one function/constructor signature
type Signature struct {
    ParameterTypes    []Type
    ReturnType        Type
    OptionalParams    []bool
    IsVariadic        bool
    RestParameterType Type
}

// Helper methods on ObjectType for clean type checking
func (ot *ObjectType) IsCallable() bool                    // Has call signatures
func (ot *ObjectType) IsConstructable() bool               // Has constructor signatures
func (ot *ObjectType) IsPureFunction() bool                // Callable with no properties
func (ot *ObjectType) GetEffectiveProperties() map[string]Type // Includes inherited
func (ot *ObjectType) GetCallSignatures() []*Signature     // Get call signatures
func (ot *ObjectType) GetConstructSignatures() []*Signature // Get constructor signatures

// Helper methods on base Type interface
func IsPrimitive(t Type) bool                               // Is Number, String, Boolean, etc.
func IsAssignable(source, target Type) bool                // Pure type assignability
func WidenType(t Type) Type                                 // Widen literals to primitives
func GetEffectiveType(t Type) Type                          // Resolve aliases, get actual type

// Constructors for common cases
func NewFunctionType(sig *Signature) *ObjectType           // Pure function
func NewOverloadedFunctionType(sigs []*Signature) *ObjectType // Overloaded function
func NewConstructorType(sig *Signature) *ObjectType        // Pure constructor
```

**Key Design Decisions:**

- ✅ **Eliminate FunctionType completely**: Everything is ObjectType like TypeScript
- ✅ **Move type operations to types package**: `IsAssignable`, `WidenType`, etc.
- ✅ **Add helper methods**: Clean `IsCallable()` instead of type switches
- ✅ **Keep `map[string]Type` for Properties**: Simple and effective

## Examples of the New System

```go
// Simple function: (x: number) => string
simpleFunc := NewFunctionType(&Signature{
    ParameterTypes: []Type{Number},
    ReturnType: String,
})
// simpleFunc.IsCallable() == true
// simpleFunc.IsPureFunction() == true

// Overloaded function: (x: number) => string & (x: string) => number
overloaded := NewOverloadedFunctionType([]*Signature{
    {ParameterTypes: []Type{Number}, ReturnType: String},
    {ParameterTypes: []Type{String}, ReturnType: Number},
})

// Complex callable object: { (): string; prop: number; method(): void }
complex := &ObjectType{
    CallSignatures: []*Signature{{ReturnType: String}},
    Properties: map[string]Type{
        "prop": Number,
        "method": NewFunctionType(&Signature{ReturnType: Void}),
    },
}
// complex.IsCallable() == true
// complex.IsPureFunction() == false

// Class with inheritance: class Derived extends Base
derived := &ObjectType{
    Properties: map[string]Type{"derivedProp": Number},
    BaseTypes: []Type{baseClassType},
}
// derived.GetEffectiveProperties() includes both derivedProp and inherited props
```

## Architectural Benefits

### 1. **Unified Type System**

- Everything is `ObjectType` - no artificial function/object distinction
- Matches TypeScript's actual internal representation
- Simpler compatibility logic - just one type to handle

### 2. **Clean Type Operations in Types Package**

```go
// Current (in checker):
func (c *Checker) isAssignable(source, target types.Type) bool { /* 600 lines */ }

// New (in types):
func IsAssignable(source, target Type) bool { /* clean, focused */ }
func (ot *ObjectType) IsAssignableTo(target Type) bool // Method form
```

### 3. **Helper Methods for Clean Code**

```go
// Current (in checker):
if funcType, ok := t.(*types.FunctionType); ok {
    // Handle function
} else if callableType, ok := t.(*types.CallableType); ok {
    // Handle callable
} else if overloadedType, ok := t.(*types.OverloadedFunctionType); ok {
    // Handle overloaded
}

// New:
if obj.IsCallable() {
    // Handle any callable thing uniformly
}
```

## Phased Implementation Plan

### Phase 1: Add Unified ObjectType + Type Operations (Foundation)

**Goal**: Add new unified ObjectType structure and move type operations to types package
**Duration**: 2-3 days
**Risk**: Low (additive changes)

#### 1.1 Extend ObjectType in `pkg/types/types.go`:

```go
// Add to ObjectType (keep existing fields for compatibility)
type ObjectType struct {
    Properties          map[string]Type     // KEEP existing
    OptionalProperties  map[string]bool     // KEEP existing

    // NEW: Unified callable/constructor support
    CallSignatures      []*Signature        // NEW: Object call signatures
    ConstructSignatures []*Signature        // NEW: Object constructor signatures
    BaseTypes           []Type              // NEW: For inheritance
}

// NEW: Signature type
type Signature struct {
    ParameterTypes    []Type
    ReturnType        Type
    OptionalParams    []bool
    IsVariadic        bool
    RestParameterType Type
}
```

#### 1.2 Add helper methods to ObjectType:

```go
func (ot *ObjectType) IsCallable() bool
func (ot *ObjectType) IsConstructable() bool
func (ot *ObjectType) IsPureFunction() bool
func (ot *ObjectType) GetCallSignatures() []*Signature
func (ot *ObjectType) GetConstructSignatures() []*Signature
func (ot *ObjectType) GetEffectiveProperties() map[string]Type  // With inheritance
func (ot *ObjectType) AddCallSignature(sig *Signature)
func (ot *ObjectType) AddConstructSignature(sig *Signature)
func (ot *ObjectType) AddBaseType(baseType Type)
```

#### 1.3 Add type-level operations to `pkg/types/types.go`:

```go
// Move from checker to types:
func IsAssignable(source, target Type) bool
func WidenType(t Type) Type
func IsPrimitive(t Type) bool
func GetEffectiveType(t Type) Type  // Resolve aliases

// Constructors for common patterns:
func NewFunctionType(sig *Signature) *ObjectType
func NewOverloadedFunctionType(sigs []*Signature) *ObjectType
func NewConstructorType(sig *Signature) *ObjectType
```

### Phase 2: Update Checker to Use Type Operations from Types Package

**Goal**: Move type logic out of checker, use new helper methods
**Duration**: 2-3 days
**Risk**: Medium (refactoring existing logic)

#### 2.1 Update `pkg/checker/type_utils.go`:

```go
// Replace 600-line isAssignable method with:
func (c *Checker) isAssignable(source, target types.Type) bool {
    return types.IsAssignable(source, target)  // Delegate to types package
}

// Remove deeplyWidenType (move to types.WidenType)
// Remove other pure type operations
```

#### 2.2 Update checker code to use helper methods:

```go
// Replace type switches with:
if objType, ok := t.(*types.ObjectType); ok && objType.IsCallable() {
    // Handle callable objects uniformly
}

// Replace complex compatibility checks with:
if types.IsAssignable(sourceType, targetType) {
    // Clean and simple
}
```

### Phase 3: Parser Integration - Generate Unified Types

**Goal**: Update parser to create ObjectType instead of separate function types
**Duration**: 2-3 days
**Risk**: Medium (changes parsing logic)

#### 3.1 Update `pkg/parser/parser.go` type expression parsing:

```go
// Replace FunctionTypeExpression parsing with ObjectType creation:
func (p *Parser) parseFunctionTypeExpression() Expression {
    // Create ObjectType with call signatures instead of FunctionType
    sig := &types.Signature{...}
    return types.NewFunctionType(sig)  // Returns ObjectType
}

func (p *Parser) parseConstructorTypeExpression() Expression {
    // Create ObjectType with constructor signatures
    sig := &types.Signature{...}
    return types.NewConstructorType(sig)  // Returns ObjectType
}
```

#### 3.2 Add inheritance parsing:

```go
func (p *Parser) parseInterfaceDeclaration() *InterfaceDeclaration {
    // Parse extends clauses and set BaseTypes
}

func (p *Parser) parseClassDeclaration() *ClassDeclaration {
    // Parse extends clause and set BaseTypes
}
```

### Phase 4: Update Type Resolution Throughout Checker

**Goal**: Checker uses unified ObjectType everywhere
**Duration**: 3-4 days
**Risk**: High (touches many files)

#### 4.1 Update `pkg/checker/function.go`:

```go
// Replace FunctionType creation with ObjectType:
func (c *Checker) resolveFunctionLiteralType(...) *types.ObjectType {
    sig := &types.Signature{...}
    return types.NewFunctionType(sig)
}
```

#### 4.2 Update `pkg/checker/call.go`:

```go
// Replace function type handling with:
func (c *Checker) checkCallExpression(node *parser.CallExpression) {
    funcType := node.Function.GetComputedType()
    if objType, ok := funcType.(*types.ObjectType); ok && objType.IsCallable() {
        // Handle all callable things uniformly
        signatures := objType.GetCallSignatures()
        // Match against signatures...
    }
}
```

#### 4.3 Update `pkg/checker/statements.go` for inheritance:

```go
func (c *Checker) checkInterfaceDeclaration(node *parser.InterfaceDeclaration) {
    // Create ObjectType with BaseTypes for inheritance
    interfaceType := &types.ObjectType{
        Properties:          properties,
        BaseTypes:          baseTypes,  // NEW
        CallSignatures:     callSigs,   // NEW
        ConstructSignatures: ctorSigs,  // NEW
    }
}
```

### Phase 5: Migrate Builtins to Unified System

**Goal**: Convert all builtin type definitions
**Duration**: 2-3 days
**Risk**: Medium (touches builtins)

#### 5.1 Update `pkg/builtins/` files:

```go
// OLD: Separate function registration
RegisterPrototypeMethod("array", "push", &types.FunctionType{...})

// NEW: Properties on ObjectType
arrayPrototype := &types.ObjectType{
    Properties: map[string]types.Type{
        "push": types.NewFunctionType(&types.Signature{...}),
        "pop":  types.NewFunctionType(&types.Signature{...}),
        // All methods become properties
    },
}
```

#### 5.2 Update `pkg/builtins/math.go`:

```go
// Convert Math object to ObjectType with function properties
mathType := &types.ObjectType{
    Properties: map[string]types.Type{
        "abs": types.NewFunctionType(&types.Signature{
            ParameterTypes: []types.Type{types.Number},
            ReturnType: types.Number,
        }),
        // ... all math functions as properties
    },
}
```

### Phase 6: Remove All Deprecated Types

**Goal**: Clean up old type system completely  
**Duration**: 1-2 days
**Risk**: Low (cleanup)

#### 6.1 Remove from `pkg/types/types.go`:

- `FunctionType` struct (replaced by ObjectType + call signatures)
- `ConstructorType` struct (replaced by ObjectType + constructor signatures)
- `CallableType` struct (replaced by ObjectType)
- `OverloadedFunctionType` struct (replaced by ObjectType + multiple signatures)

#### 6.2 Update all remaining references:

- Compiler usage of old function types
- Any remaining type switches in checker
- Test files

### Phase 7: Integration Testing & Validation

**Goal**: Ensure unified system works correctly
**Duration**: 1-2 days  
**Risk**: Low (testing)

#### 7.1 Comprehensive testing:

- All existing test suites should pass
- Test inheritance scenarios (class/interface extends)
- Test complex callable objects
- Test overloaded functions
- Test TypeScript-style interfaces

#### 7.2 Add new test cases for unified system:

```typescript
// These should all work after refactoring:

// Simple function
type SimpleFunc = (x: number) => string;

// Overloaded function
interface OverloadedFunc {
  (x: number): string;
  (x: string): number;
}

// Callable object with properties
interface CallableObject {
  (x: string): void;
  count: number;
  reset(): void;
}

// Complex inheritance
interface Base {
  baseProp: string;
  baseMethod(): void;
}

interface Derived extends Base {
  derivedProp: number;
  (x: string): number; // call signature
  new (): Derived; // constructor signature
}

// Class inheritance
class BaseClass {
  baseProp: string = "";
  baseMethod(): void {}
}

class DerivedClass extends BaseClass {
  derivedProp: number = 0;
  derivedMethod(): string {
    return "";
  }
}
```

## Implementation Strategy

### Development Approach:

1. **Backward compatibility first**: Keep old types during transition
2. **Move type operations early**: Get clean separation of concerns
3. **Incremental migration**: Each phase builds on previous
4. **Use helper methods**: Clean up checker code significantly
5. **Comprehensive testing**: Validate each phase thoroughly

### Key Risk Mitigations:

1. **Gradual migration**: Don't break existing functionality
2. **Type operation testing**: Ensure `IsAssignable` etc. work correctly
3. **Helper method validation**: Test `IsCallable()`, `IsPureFunction()` etc.
4. **Inheritance testing**: Validate base type merging
5. **Performance monitoring**: Ensure unified approach isn't slower

### Testing Strategy:

1. **Unit tests**: Test each helper method and type operation
2. **Type operation tests**: Validate assignability, widening, etc.
3. **Inheritance tests**: Test base type merging and property resolution
4. **Integration tests**: Full parser → checker → compiler pipeline
5. **Regression tests**: All existing functionality continues working
6. **TypeScript compatibility**: Match TypeScript interface semantics

## Expected Benefits

1. **Truly unified type system**: Everything is ObjectType like TypeScript
2. **Clean architecture**: Type operations in types package, not checker
3. **Better helper methods**: `IsCallable()` instead of complex type switches
4. **Proper inheritance**: Class and interface inheritance with member merging
5. **Simplified compatibility**: One type system instead of cross-type bridges
6. **TypeScript compatibility**: Can represent any TypeScript interface
7. **Easier maintenance**: Less complex code, focused responsibilities
8. **Performance**: Potentially faster with unified approach and helper methods

## Files Requiring Changes

### High Impact (Major Changes):

- `pkg/types/types.go` - Add unified ObjectType + type operations
- `pkg/checker/type_utils.go` - Remove ~600 lines, delegate to types
- `pkg/checker/statements.go` - Use unified types + inheritance
- `pkg/checker/call.go` - Use IsCallable() helper methods

### Medium Impact (Moderate Changes):

- `pkg/parser/parser.go` - Generate ObjectType instead of FunctionType
- `pkg/checker/function.go` - Return ObjectType from type resolution
- `pkg/checker/expressions.go` - Use helper methods
- `pkg/builtins/*.go` - Convert to ObjectType with properties
- `pkg/compiler/*.go` - Use unified types

### Low Impact (Minor Updates):

- Test files - Update for unified type system
- Documentation - Update for new architecture

## Timeline Estimate

- **Phase 1**: 3 days (Foundation + type operations move)
- **Phase 2**: 3 days (Update checker to use new operations)
- **Phase 3**: 3 days (Parser generates unified types)
- **Phase 4**: 4 days (Update checker for unified ObjectType)
- **Phase 5**: 3 days (Migrate builtins)
- **Phase 6**: 2 days (Remove deprecated types)
- **Phase 7**: 2 days (Testing + validation)
- **Buffer**: 2 days (Unexpected issues)

**Total**: ~3 weeks for complete unified type system

This approach creates a truly unified, TypeScript-compatible type system with clean architecture and helpful abstractions for easier type checking.

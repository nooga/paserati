# Prototypal Inheritance Implementation Plan

## Executive Summary

This document outlines a comprehensive, phased implementation plan for adding prototypal inheritance to Paserati. The goal is to achieve both **correctness** (full JavaScript semantics) and **performance** (competitive with modern JS engines) through careful design of inline caches, shape-based optimizations, and prototype chain handling.

## Current State Analysis

### ✅ **What We Already Have (Strong Foundation)**

1. **Shape-Based Object Model**

   - `Shape` struct with field transitions for monomorphic optimization
   - `PlainObject` with shape-based property storage for cache-friendly access
   - Shape transitions cached in `transitions` map for property addition

2. **Advanced Inline Cache System**

   - `PropInlineCache` with monomorphic/polymorphic/megamorphic states
   - Cache entries with `shape + offset` for O(1) property access
   - Cache hit/miss statistics and performance monitoring
   - Hash-based cache keys to avoid instruction pointer collisions

3. **Constructor Infrastructure**

   - `OpNew` implementation with proper `this` binding
   - `CallFrame.thisValue` and `CallFrame.isConstructorCall`
   - Method calls with `OpCallMethod` and context binding

4. **Property Access Foundation**
   - `OpGetProp`/`OpSetProp` with constant pool property names
   - Prototype-aware property access for built-in methods (String/Array prototypes)
   - `GetOwn`/`SetOwn` methods that operate only on own properties

### ❌ **What We're Missing (Critical Gaps)**

1. **Prototype Chain Traversal**

   - Property lookup only checks own properties (`GetOwn`)
   - No `[[Prototype]]` chain walking in `OpGetProp`
   - Missing prototype inheritance in property resolution

2. **Constructor-Prototype Relationship**

   - Functions don't automatically get a `.prototype` property
   - `OpNew` doesn't set `instance.__proto__ = Constructor.prototype`
   - No automatic prototype assignment for instances

3. **`instanceof` Operator**

   - Missing `OpInstanceof` bytecode instruction
   - No prototype chain traversal for instanceof checks

4. **Prototype-Aware Inline Caches**
   - Current ICs only cache own property lookups
   - No prototype chain caching (critical for performance)
   - Missing method resolution caching for inherited methods

## Performance Implications & Strategy

### **Inline Cache Challenges**

The current IC system assumes `shape + property_name → offset` for own properties. Prototypes introduce additional complexity:

```javascript
function Person(name) {
  this.name = name;
}
Person.prototype.greet = function () {
  return "Hello " + this.name;
};

let john = new Person("John");
john.greet(); // Method inherited from Person.prototype
```

**Challenge**: `john.greet` lookup needs to:

1. Check `john` shape for own property "greet" (miss)
2. Check `john.__proto__` (Person.prototype) for "greet" (hit)
3. Cache the full lookup path for subsequent calls

### **Proposed IC Extensions**

#### **1. Prototype Chain Cache Entries**

```go
type ProtoCacheEntry struct {
    objectShape    *Shape  // Shape of the object being accessed
    prototypeShape *Shape  // Shape where property was found
    prototypeDepth int     // How many steps up the chain (0=own, 1=proto, 2=proto.proto)
    offset         int     // Offset in the prototype object
    boundMethod    Value   // Cached bound method (if applicable)
}
```

#### **2. Extended Inline Cache**

```go
type PropInlineCache struct {
    state           PropCacheState
    entries         [4]PropCacheEntry    // Own property cache (existing)
    protoEntries    [4]ProtoCacheEntry   // Prototype chain cache (new)
    protoEntryCount int
    // ... existing fields
}
```

#### **3. Cache Lookup Algorithm**

```go
func (ic *PropInlineCache) lookupInCache(objShape *Shape, objProto Value) (CacheResult, bool) {
    // 1. Try own property cache first (fast path)
    if offset, hit := ic.lookupOwnProperty(objShape); hit {
        return OwnPropertyResult{offset: offset}, true
    }

    // 2. Try prototype chain cache
    for i := 0; i < ic.protoEntryCount; i++ {
        entry := ic.protoEntries[i]
        if entry.objectShape == objShape {
            // Validate prototype chain hasn't changed
            if validatePrototypeChain(objProto, entry.prototypeShape, entry.prototypeDepth) {
                return PrototypeResult{
                    protoValue: getPrototypeAtDepth(objProto, entry.prototypeDepth),
                    offset: entry.offset,
                    boundMethod: entry.boundMethod,
                }, true
            }
        }
    }

    return CacheResult{}, false
}
```

### **Performance Optimizations**

1. **Shape Stability Assumptions**

   - Prototype objects rarely change shape after initialization
   - Cache prototype shapes aggressively
   - Invalidate prototype caches only when prototype shapes change

2. **Method Binding Cache**

   - Cache bound methods (`this + function`) to avoid repeated binding
   - Key: `(this_value, method_function) → bound_method`

3. **Prototype Chain Stability**

   - Most objects have shallow prototype chains (1-2 levels)
   - Cache the full chain validation, not just individual steps

4. **Lazy Prototype Initialization** - **NEW OPTIMIZATION**
   - Functions start without `.prototype` property to save memory
   - Create prototype object only when first accessed or when used as constructor
   - Significant memory savings for utility functions, callbacks, etc.

## Type Checker Integration

### **✅ Existing Type System Strengths**

Looking at `pkg/types/object.go`, Paserati's type system is **already well-prepared** for inheritance:

```go
type ObjectType struct {
    Properties         map[string]Type
    OptionalProperties map[string]bool
    CallSignatures      []*Signature  // Object call signatures: obj(args)
    ConstructSignatures []*Signature  // Object constructor signatures: new obj(args)
    BaseTypes           []Type        // 🎯 Already supports inheritance!
}

// 🎯 Already have inheritance resolution!
func (ot *ObjectType) GetEffectiveProperties() map[string]Type {
    // Merges properties from base types with proper precedence
}
```

**This means**: The type checker already understands inheritance chains, we just need to **connect runtime prototypes to compile-time types**.

### **Type Checker Impact Analysis**

#### **1. Constructor Function Types** - **Moderate Changes**

**Current state**: Functions are typed as `ObjectType` with call signatures
**Needed**: Functions need a `.prototype` property type

```go
// Before: function Person(name: string): void
funcType := NewSimpleFunction([]Type{String}, Void)

// After: function Person(name: string): void & { prototype: PersonPrototype }
constructorType := NewObjectType().
    WithSimpleCallSignature([]Type{String}, Void).  // Callable as function
    WithSimpleConstructSignature([]Type{String}, PersonInstanceType).  // Callable with 'new'
    WithProperty("prototype", PersonPrototypeType)  // Has .prototype property
```

#### **2. Prototype Chain Type Resolution** - **Minor Changes**

**Current**: `GetPropertyType()` in `pkg/types/props.go` already handles inheritance
**Needed**: Map runtime prototype chains to type inheritance chains

```go
// Current inheritance resolution already works:
function GetPropertyType(objectType Type, propertyName string) Type {
    switch obj := objectType.(type) {
    case *ObjectType:
        // Already checks obj.GetEffectiveProperties() which walks BaseTypes!
    }
}
```

**Integration point**: When we resolve `john.greet`, we need to:

1. Check `john`'s type for own property "greet"
2. Walk the **type inheritance chain** (using existing `BaseTypes`)
3. This mirrors the **runtime prototype chain** we'll implement

#### **3. instanceof Type Checking** - **New Feature**

**Needed**: `instanceof` operator type checking

```go
// In checker: obj instanceof Constructor
func (c *Checker) checkInstanceofExpression(node *parser.BinaryExpression) {
    objType := c.getType(node.Left)
    constructorType := c.getType(node.Right)

    // Get constructor's instance type from ConstructSignatures
    if ctorObj, ok := constructorType.(*ObjectType); ok && ctorObj.IsConstructable() {
        instanceType := ctorObj.GetConstructSignatures()[0].ReturnType

        // Check if objType is assignable to instanceType
        if c.isAssignableTo(objType, instanceType) {
            // instanceof will be true
        }
    }

    // Result is always boolean
    node.SetComputedType(Boolean)
}
```

#### **4. Prototype Property Assignment** - **New Feature**

**Needed**: Type checking for `Constructor.prototype.method = function() {}`

```go
// Constructor.prototype.method = function() {}
// Need to:
// 1. Resolve Constructor.prototype type
// 2. Check method assignment is valid
// 3. Update the prototype type's properties
```

### **Implementation Strategy for Type Checker**

#### **Phase 1: Constructor Function Types**

**File**: `pkg/checker/functions.go`

```go
func (c *Checker) checkFunctionDeclaration(node *parser.FunctionDeclaration) {
    // Existing function type creation...
    funcType := c.createFunctionType(node)

    // NEW: Add .prototype property type for constructor functions
    if c.couldBeConstructor(node) {
        prototypeType := c.createPrototypeType(node)

        // Enhance function type with .prototype property
        if objType, ok := funcType.(*ObjectType); ok {
            objType.WithProperty("prototype", prototypeType)
            // Also add constructor signature
            instanceType := c.createInstanceType(node)
            objType.WithConstructSignature(c.createConstructorSignature(node, instanceType))
        }
    }
}
```

#### **Phase 2: Instance Type Creation**

```go
func (c *Checker) createInstanceType(constructorNode *parser.FunctionDeclaration) Type {
    // Create instance type based on:
    // 1. Properties assigned in constructor body (this.prop = value)
    // 2. Properties assigned to Constructor.prototype

    instanceType := NewObjectType()

    // Analyze constructor body for this.prop assignments
    c.analyzeConstructorBody(constructorNode, instanceType)

    // Set up inheritance: instance.__proto__ = Constructor.prototype
    prototypeType := c.getOrCreatePrototypeType(constructorNode)
    instanceType.Inherits(prototypeType)  // 🎯 Use existing inheritance!

    return instanceType
}
```

#### **Phase 3: Prototype Chain Resolution**

The existing `GetEffectiveProperties()` method already handles this correctly:

```go
// This already works with inheritance chains!
func GetPropertyType(objectType Type, propertyName string) Type {
    switch obj := objectType.(type) {
    case *ObjectType:
        // GetEffectiveProperties() walks BaseTypes automatically
        props := obj.GetEffectiveProperties()
        if propType, exists := props[propertyName]; exists {
            return propType
        }
    }
    return Never
}
```

**Key insight**: We just need to ensure runtime prototype chains match compile-time `BaseTypes` chains.

### **Type Checker Integration Summary**

| Feature                   | Current State                                                | Changes Needed                       | Complexity |
| ------------------------- | ------------------------------------------------------------ | ------------------------------------ | ---------- |
| **Object inheritance**    | ✅ Fully implemented (`BaseTypes`, `GetEffectiveProperties`) | None                                 | None       |
| **Property resolution**   | ✅ Works with inheritance                                    | Map runtime prototypes to types      | Low        |
| **Constructor functions** | ✅ Call signatures exist                                     | Add `.prototype` property type       | Medium     |
| **instanceof checking**   | ❌ Missing                                                   | Type checking for instanceof         | Medium     |
| **Prototype assignment**  | ❌ Missing                                                   | `Constructor.prototype.prop = value` | Medium     |

**The type system is remarkably well-prepared!** The existing inheritance infrastructure (`BaseTypes`, `GetEffectiveProperties`) maps perfectly to JavaScript's prototype chains.

**Main work**:

1. **Connect** runtime prototype objects to compile-time inheritance types
2. **Extend** constructor function types to include `.prototype` property
3. **Add** `instanceof` type checking

**Estimated type checker work**: ~20% of total prototypal inheritance effort, since the core inheritance system already exists.

## Implementation Plan

### **Phase 1: Foundation (Correctness Focus)**

**Goal**: Get prototypal inheritance working correctly, performance optimizations come later.

#### **1.1 Extend Object Model**

**File**: `pkg/vm/object.go`

```go
// Add prototype chain traversal methods
func (o *PlainObject) Get(name string) (Value, bool) {
    // Check own properties first
    if value, exists := o.GetOwn(name); exists {
        return value, true
    }

    // Walk prototype chain
    current := o.prototype
    for !current.IsNull() && !current.IsUndefined() {
        if current.IsObject() {
            if proto := current.AsPlainObject(); proto != nil {
                if value, exists := proto.GetOwn(name); exists {
                    return value, true
                }
                current = proto.prototype
            } else {
                break
            }
        } else {
            break
        }
    }

    return Undefined, false
}

func (o *PlainObject) Has(name string) bool {
    _, exists := o.Get(name)
    return exists
}

// Add prototype getter/setter for debugging
func (o *PlainObject) GetPrototype() Value {
    return o.prototype
}

func (o *PlainObject) SetPrototype(proto Value) {
    o.prototype = proto
    // TODO: Invalidate related caches
}
```

**DictObject equivalent methods** needed as well.

#### **1.2 Update Property Access in VM**

**File**: `pkg/vm/vm.go` (OpGetProp case)

```go
case OpGetProp:
    // ... existing setup code ...

    // Replace the current property lookup with prototype-aware version
    if objVal.Type() == TypeObject {
        po := AsPlainObject(objVal)

        // Use new Get method instead of GetOwn
        if fv, ok := po.Get(propName); ok {
            registers[destReg] = fv
        } else {
            registers[destReg] = Undefined
        }
        continue
    }

    // Similar changes for DictObject
```

**Note**: This breaks existing inline caches temporarily, but establishes correct semantics.

#### **1.3 Add Lazy Constructor.prototype Property** - **MEMORY OPTIMIZED**

**File**: `pkg/vm/function.go`

**Key insight**: Don't create `.prototype` until accessed or used as constructor.

```go
// FunctionObject stays lean - no automatic prototype creation
type FunctionObject struct {
    Object
    Arity        int
    Variadic     bool
    Chunk        *Chunk
    Name         string
    UpvalueCount int
    RegisterSize int
    Properties   *PlainObject  // For properties like .prototype (created lazily)
}

// NewFunction - no automatic prototype creation
func NewFunction(arity, upvalueCount, registerSize int, variadic bool, name string, chunk *Chunk) Value {
    fnObj := &FunctionObject{
        Arity:        arity,
        Variadic:     variadic,
        Chunk:        chunk,
        Name:         name,
        UpvalueCount: upvalueCount,
        RegisterSize: registerSize,
        Properties:   nil, // Start with nil - create lazily
    }

    return Value{typ: TypeFunction, obj: unsafe.Pointer(fnObj)}
}

// Helper function to get or create function prototype
func (fn *FunctionObject) getOrCreatePrototype() Value {
    // Ensure Properties object exists
    if fn.Properties == nil {
        fn.Properties = NewObject(Undefined).AsPlainObject()
    }

    // Check if prototype already exists
    if proto, exists := fn.Properties.GetOwn("prototype"); exists {
        return proto
    }

    // Create prototype lazily
    prototypeObj := NewObject(DefaultObjectPrototype)
    fn.Properties.SetOwn("prototype", prototypeObj)

    // Set constructor property on prototype (circular reference)
    if prototypeObj.IsObject() {
        protoPlain := prototypeObj.AsPlainObject()
        constructorVal := Value{typ: TypeFunction, obj: unsafe.Pointer(fn)}
        protoPlain.SetOwn("constructor", constructorVal)
    }

    return prototypeObj
}
```

#### **1.4 Handle Lazy Prototype in Property Access**

**File**: `pkg/vm/vm.go` (OpGetProp case for functions)

```go
// Handle property access on functions (including lazy .prototype)
if objVal.Type() == TypeFunction {
    fn := AsFunction(objVal)

    // Special handling for "prototype" property
    if propName == "prototype" {
        registers[destReg] = fn.getOrCreatePrototype()
        continue
    }

    // Other function properties (if any)
    if fn.Properties != nil {
        if prop, exists := fn.Properties.GetOwn(propName); exists {
            registers[destReg] = prop
            continue
        }
    }

    registers[destReg] = Undefined
    continue
}
```

#### **1.5 Fix OpNew with Lazy Prototype Handling**

**File**: `pkg/vm/vm.go` (OpNew case)

```go
case OpNew:
    // ... existing constructor setup ...

    // Get constructor's .prototype property (create lazily if needed)
    var instancePrototype Value = DefaultObjectPrototype

    if constructorVal.Type() == TypeFunction {
        fn := AsFunction(constructorVal)
        instancePrototype = fn.getOrCreatePrototype()
    } else if constructorVal.Type() == TypeClosure {
        closure := AsClosure(constructorVal)
        instancePrototype = closure.Fn.getOrCreatePrototype()
    }

    // Create new instance with correct prototype
    newInstance := NewObject(instancePrototype)

    // ... rest of existing OpNew logic
```

#### **1.6 Add instanceof Operator**

**New opcode**: `OpInstanceof`

**File**: `pkg/vm/bytecode.go`

```go
OpInstanceof OpCode = 61 // Rx Ry Rz: Rx = (Ry instanceof Rz)
```

**File**: `pkg/vm/vm.go`

```go
case OpInstanceof:
    destReg := code[ip]
    objReg := code[ip+1]
    constructorReg := code[ip+2]
    ip += 3

    objVal := registers[objReg]
    constructorVal := registers[constructorReg]

    // Get constructor's .prototype property (may create it lazily)
    var constructorPrototype Value = Undefined
    if constructorVal.Type() == TypeFunction {
        fn := AsFunction(constructorVal)
        constructorPrototype = fn.getOrCreatePrototype()
    } else if constructorVal.Type() == TypeClosure {
        closure := AsClosure(constructorVal)
        constructorPrototype = closure.Fn.getOrCreatePrototype()
    }

    // Walk prototype chain of object
    result := false
    if objVal.IsObject() {
        current := objVal.AsPlainObject().GetPrototype()
        for !current.IsNull() && !current.IsUndefined() {
            if current.Equals(constructorPrototype) {
                result = true
                break
            }
            if current.IsObject() {
                current = current.AsPlainObject().GetPrototype()
            } else {
                break
            }
        }
    }

    registers[destReg] = BooleanValue(result)
```

#### **1.7 Parser & Compiler Support**

**File**: `pkg/parser/parser.go` - Add instanceof as binary operator
**File**: `pkg/compiler/compile_expression.go` - Compile instanceof expressions

### **Phase 2: Inline Cache Integration (Performance Focus)**

**Goal**: Restore and enhance inline cache performance for prototype lookups.

#### **2.1 Prototype-Aware Cache Structure**

**File**: `pkg/vm/vm.go`

```go
type PrototypeCacheEntry struct {
    objectShape     *Shape
    prototypeChain  []*Shape // Shapes of objects in prototype chain
    chainDepth      int      // How deep the property was found
    propertyOffset  int      // Offset in the target prototype object
    boundMethod     Value    // Cached bound method (for functions)
    chainValid      bool     // Quick validation flag
}

type PropInlineCache struct {
    state               PropCacheState
    ownEntries         [4]PropCacheEntry     // Own property cache
    prototypeEntries   [4]PrototypeCacheEntry // Prototype chain cache
    ownEntryCount      int
    prototypeEntryCount int
    hitCount           uint32
    missCount          uint32
}
```

#### **2.2 Enhanced Cache Lookup**

```go
func (ic *PropInlineCache) lookupInCache(objShape *Shape, objProto Value, propName string) (Value, bool) {
    // 1. Try own property cache (unchanged from current implementation)
    if offset, hit := ic.lookupOwnProperty(objShape); hit {
        return getCachedOwnProperty(objShape, offset), true
    }

    // 2. Try prototype cache
    for i := 0; i < ic.prototypeEntryCount; i++ {
        entry := &ic.prototypeEntries[i]
        if entry.objectShape == objShape && entry.chainValid {
            // Quick validation: check if prototype chain shapes are still valid
            if ic.validatePrototypeChain(objProto, entry) {
                // Cache hit!
                if entry.boundMethod.IsUndefined() {
                    // Regular property
                    return ic.getPrototypeProperty(objProto, entry), true
                } else {
                    // Cached bound method
                    return entry.boundMethod, true
                }
            } else {
                // Prototype chain changed, invalidate this entry
                entry.chainValid = false
            }
        }
    }

    return Undefined, false
}
```

#### **2.3 Cache Population Strategy**

```go
func (ic *PropInlineCache) updatePrototypeCache(objShape *Shape, objProto Value, propName string, foundValue Value, chainDepth int) {
    if ic.prototypeEntryCount >= 4 {
        // Transition to megamorphic for prototype lookups
        return
    }

    entry := &ic.prototypeEntries[ic.prototypeEntryCount]
    entry.objectShape = objShape
    entry.chainDepth = chainDepth
    entry.chainValid = true

    // Cache bound method if it's a function
    if foundValue.IsFunction() || foundValue.IsNativeFunction() {
        entry.boundMethod = createBoundMethod(getCurrentObjectValue(), foundValue)
    } else {
        entry.boundMethod = Undefined
    }

    // Store prototype chain shapes for validation
    ic.capturePrototypeChain(objProto, entry, chainDepth)

    ic.prototypeEntryCount++
}
```

### **Phase 3: Advanced Optimizations**

#### **3.1 Prototype Shape Stability**

- **Assumption**: Prototype objects rarely change after initialization
- **Strategy**: Cache prototype shapes globally, invalidate on rare changes
- **Implementation**: Global prototype shape registry with version numbers

#### **3.2 Method Resolution Cache**

- **Problem**: Bound method creation is expensive
- **Solution**: Global cache of `(this_type, method_function) → bound_method`
- **Invalidation**: When prototype methods change (rare)

#### **3.3 Constructor Prototype Caching**

- **Problem**: `Constructor.prototype` lookup on every `new` call
- **Solution**: Cache constructor prototypes by function identity
- **Key insight**: Function objects rarely change their .prototype property

#### **3.4 Memory Usage Monitoring** - **NEW**

With lazy prototype initialization, monitor memory savings:

```go
type MemoryStats struct {
    functionsCreated       int64
    prototypesCreated      int64  // Should be much lower
    prototypeLazyHits      int64  // How often we avoided creation
    avgFunctionSize        int64
    avgPrototypeSize       int64
}

func (vm *VM) GetMemoryStats() MemoryStats {
    // Track prototype creation vs function creation ratios
    // Goal: < 30% of functions should need prototypes
}
```

### **Phase 4: Built-in Integration**

#### **4.1 Standard Library Methods**

Add prototype chain support to built-in objects:

- `Object.create()`, `Object.getPrototypeOf()`, `Object.setPrototypeOf()`
- `Object.prototype.hasOwnProperty()`, `Object.prototype.isPrototypeOf()`
- `Function.prototype.call()`, `Function.prototype.apply()`, `Function.prototype.bind()`

#### **4.2 Error Object Hierarchy**

Implement prototype-based error hierarchy:

- `Error.prototype` as base
- `TypeError.prototype.__proto__ = Error.prototype`
- `ReferenceError.prototype.__proto__ = Error.prototype`

### **Phase 5: Class Syntax Sugar**

#### **5.1 Class Declaration/Expression Parsing**

```typescript
class Person {
  constructor(name) {
    this.name = name;
  }
  greet() {
    return "Hello " + this.name;
  }
  static species() {
    return "Homo sapiens";
  }
}
```

#### **5.2 Class Compilation Strategy**

Classes compile to:

1. Constructor function
2. Prototype method assignment
3. Static method assignment
4. Proper prototype chain setup

```javascript
// Class compiles to roughly:
function Person(name) {
  this.name = name;
}
Person.prototype.greet = function () {
  return "Hello " + this.name;
};
Person.species = function () {
  return "Homo sapiens";
};
```

#### **5.3 Inheritance (extends)**

```typescript
class Student extends Person {
  constructor(name, school) {
    super(name);
    this.school = school;
  }
  study() {
    return this.name + " studies at " + this.school;
  }
}
```

Requires:

- `super()` call compilation
- Prototype chain setup: `Student.prototype.__proto__ = Person.prototype`
- Static inheritance: `Student.__proto__ = Person`

## Memory Efficiency Benefits

### **Lazy Prototype Benefits**

1. **Utility Functions**: Functions used only as utilities (map callbacks, event handlers) never create prototypes
2. **Arrow Functions**: Already can't be constructors, so never need prototypes
3. **Built-in Methods**: Array/String methods don't need user-accessible prototypes
4. **Short-lived Functions**: Temporary functions in closures avoid prototype overhead

### **Expected Memory Savings**

```javascript
// These functions never need prototypes:
let arr = [1, 2, 3].map((x) => x * 2); // Arrow function - no prototype
let filtered = arr.filter(function (x) {
  return x > 2;
}); // Utility - no prototype
setTimeout(function () {
  console.log("hi");
}, 1000); // Callback - no prototype

// Only these need prototypes:
function Person(name) {
  this.name = name;
} // Constructor - creates prototype on first `new Person()`
let john = new Person("John"); // Triggers prototype creation

// Or explicit access:
Person.prototype.greet = function () {}; // Triggers prototype creation
```

**Estimated savings**: 60-80% reduction in function-related object allocations.

## Testing Strategy

### **Phase 1 Tests**

```typescript
// Basic prototype chain
function Person(name) {
  this.name = name;
}
Person.prototype.greet = function () {
  return "Hello " + this.name;
};
let john = new Person("John");
john.greet(); // "Hello John"

// instanceof
john instanceof Person; // true
john instanceof Object; // true

// Prototype chain traversal
john.hasOwnProperty; // inherited from Object.prototype

// Memory efficiency tests
function utility(x) {
  return x * 2;
}
// utility should NOT have a .prototype property created
// Only after: utility.prototype or new utility() should prototype exist
```

### **Phase 2 Tests**

```typescript
// IC performance with repeated access
let people = [new Person("Alice"), new Person("Bob"), new Person("Charlie")];
for (let person of people) {
  person.greet(); // Should hit prototype cache after first miss
}

// Polymorphic prototype caches
function Animal(name) {
  this.name = name;
}
Animal.prototype.speak = function () {
  return this.name + " makes a sound";
};

function Dog(name) {
  this.name = name;
}
Dog.prototype.speak = function () {
  return this.name + " barks";
};

let animals = [new Animal("Generic"), new Dog("Rex")];
animals.forEach((a) => a.speak()); // Tests polymorphic prototype cache
```

### **Phase 3 Performance Tests**

- Micro-benchmarks vs Node.js for common prototype patterns
- Memory usage analysis for prototype cache overhead
- Cache hit rate monitoring for real-world patterns
- **Memory efficiency tests**: Verify 60-80% reduction in prototype object creation

## Migration Path

### **Breaking Changes (Minimal)**

1. Functions now have a `.prototype` property **when accessed** (enhancement)
2. `new Constructor()` creates instances with proper prototype chain (enhancement)
3. Property access now walks prototype chain (behavioral change, mostly compatible)

### **Backwards Compatibility**

- Existing object literal code continues to work
- Built-in prototype methods remain accessible
- Performance should improve for most patterns
- **Memory usage significantly reduced** for function-heavy code

### **Feature Flags**

Consider adding a compiler flag `--legacy-prototypes` that disables prototype chain traversal for debugging migration issues.

## Risk Analysis

### **High Risk**

1. **Performance Regression**: Prototype lookups are inherently slower than own-property lookups
   - **Mitigation**: Extensive IC caching, benchmark against current performance
2. **Cache Complexity**: Prototype-aware caches are much more complex
   - **Mitigation**: Thorough testing, gradual rollout, fallback to slow path

### **Medium Risk**

1. **Memory Usage**: Prototype caches consume additional memory
   - **Mitigation**: Cache size limits, LRU eviction policies
   - **Counter-benefit**: Lazy prototype initialization saves much more memory
2. **Cache Invalidation**: Prototype changes must invalidate many caches
   - **Mitigation**: Rare in practice, acceptable slow path for rare cases

### **Low Risk**

1. **Correctness**: Well-defined JavaScript semantics for prototypes
   - **Mitigation**: Comprehensive test suite against Node.js behavior
2. **Lazy Prototype Logic**: Simple conditional creation pattern
   - **Mitigation**: Straightforward implementation, easy to test

## Success Metrics

### **Correctness**

- [ ] All prototype chain lookups work correctly
- [ ] `instanceof` operator works for all cases
- [ ] Class syntax compiles and executes correctly
- [ ] Built-in prototype methods accessible on primitives

### **Performance**

- [ ] Prototype cache hit rate > 95% for typical workloads
- [ ] Method calls on instances no more than 20% slower than current
- [ ] Constructor calls no more than 30% slower than current
- [ ] Memory overhead < 15% for typical object-heavy programs

### **Memory Efficiency** - **NEW METRICS**

- [ ] 60-80% reduction in function prototype object creation
- [ ] Prototype creation rate < 30% of function creation rate
- [ ] Memory savings measurable in real-world TypeScript applications
- [ ] No performance penalty for prototype access (lazy creation should be fast)

### **Compatibility**

- [ ] Existing test suite passes without modification
- [ ] Real-world TypeScript/JavaScript code patterns work correctly
- [ ] Performance competitive with Node.js for prototype-heavy code

## Implementation Timeline

- **Phase 1**: 2-3 weeks (foundation correctness + lazy prototypes)
- **Phase 2**: 2-3 weeks (inline cache integration)
- **Phase 3**: 1-2 weeks (advanced optimizations + memory monitoring)
- **Phase 4**: 1-2 weeks (standard library)
- **Phase 5**: 2-3 weeks (class syntax)

**Total**: ~8-13 weeks for complete prototypal inheritance system

---

_This plan balances correctness, performance, and memory efficiency while building on Paserati's existing strengths in shape-based optimization and inline caching. The lazy prototype initialization provides significant memory savings while maintaining full JavaScript compatibility. The phased approach allows for iterative validation and performance tuning._

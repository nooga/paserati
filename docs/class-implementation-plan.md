# Class Implementation Plan

This document outlines the implementation of TypeScript/JavaScript class support in Paserati, following a phased approach that builds upon the existing prototype system and constructor functions.

## 🎯 Overview

Classes are fundamental to modern TypeScript development. By implementing classes as syntactic sugar over Paserati's existing prototype system, we can provide full class functionality while leveraging proven infrastructure.

## 🚀 Implementation Strategy

### Core Approach: Desugar to Constructor Functions + Prototypes

Classes will be compiled to equivalent constructor function + prototype patterns:

```typescript
// Input TypeScript
class Animal {
  name: string;
  
  constructor(name: string) {
    this.name = name;
  }
  
  speak(): string {
    return `${this.name} makes a sound`;
  }
}
```

```javascript
// Compiled Output (conceptual)
function Animal(name) {
  this.name = name;
}

Animal.prototype.speak = function() {
  return this.name + " makes a sound";
};
```

### Benefits of This Approach

1. **Leverages existing infrastructure** - Uses proven prototype system
2. **JavaScript compatible** - Produces standard prototype-based objects  
3. **Incremental implementation** - Can build features progressively
4. **Zero runtime overhead** - No additional abstraction layer
5. **TypeScript compliant** - Matches TypeScript's compilation model

## 📋 Implementation Phases

### Phase 1: Basic Class Declaration ⏳ **PLANNED**

**Goal**: Support basic class syntax with constructor, fields, and methods.

#### 1.1 Lexer Extensions
```go
// Add to lexer/token.go
CLASS      // class keyword
EXTENDS    // extends keyword (for future inheritance)
STATIC     // static keyword (for future static members)
```

#### 1.2 Parser Extensions
```go
// Add to parser/ast.go
type ClassDeclaration struct {
    BaseStatement
    Name        *Identifier
    SuperClass  *Identifier          // nil for basic classes
    Body        *ClassBody
}

type ClassExpression struct {
    BaseExpression
    Name        *Identifier          // nil for anonymous classes
    SuperClass  *Identifier          // nil for basic classes
    Body        *ClassBody
}

type ClassBody struct {
    Methods     []*MethodDefinition
    Properties  []*PropertyDefinition
}

type MethodDefinition struct {
    BaseExpression
    Key         *Identifier
    Value       *FunctionExpression
    Kind        string               // "constructor", "method"
    IsStatic    bool
}

type PropertyDefinition struct {
    BaseExpression  
    Key         *Identifier
    Value       Expression           // initializer, can be nil
    IsStatic    bool
}
```

#### 1.3 Parser Logic
```go
// Add to parser/parser.go
func (p *Parser) parseClassDeclaration() Statement {
    // Parse: class ClassName { body }
    // Handle constructor, methods, properties
}

func (p *Parser) parseClassExpression() Expression {
    // Parse: class [ClassName] { body }
    // Handle anonymous classes: class { ... }
    // Handle named class expressions: class MyClass { ... }
}

func (p *Parser) parseClassBody() *ClassBody {
    // Parse methods and property definitions
    // Distinguish constructor from regular methods
}
```

#### 1.4 Integration with Parsing
```go
// Extend parseStatement() to handle CLASS token (declarations)
case lexer.CLASS:
    return p.parseClassDeclaration()

// Extend parsePrimaryExpression() to handle CLASS token (expressions)
case lexer.CLASS:
    return p.parseClassExpression()
```

### Phase 2: Type System Integration ⏳ **PLANNED**

**Goal**: Add class types to the type checker with proper inheritance support.

#### 2.1 Type Definitions
```go
// Add to types/class.go
type ClassType struct {
    Name            string
    ConstructorType *FunctionType     // Constructor signature
    InstanceType    *ObjectType       // Instance shape
    StaticType      *ObjectType       // Static methods/properties
    SuperClass      *ClassType        // For inheritance
}

func NewClassType(name string, ctor *FunctionType, instance *ObjectType) *ClassType
```

#### 2.2 Type Checking
```go
// Add to checker/check_class.go
func (c *Checker) checkClassDeclaration(node *parser.ClassDeclaration) Type {
    // 1. Create instance type from methods and properties
    instanceType := c.createInstanceType(node.Body)
    
    // 2. Create constructor type
    ctorType := c.createConstructorType(node.Body.Constructor, instanceType)
    
    // 3. Create class type
    classType := types.NewClassType(node.Name.Value, ctorType, instanceType)
    
    // 4. Register in environment
    c.env.Define(node.Name.Value, classType)
    
    return classType
}

func (c *Checker) createInstanceType(body *parser.ClassBody) *types.ObjectType {
    // Build object type from methods and properties
    objType := types.NewObjectType()
    
    for _, method := range body.Methods {
        if method.Kind != "constructor" && !method.IsStatic {
            methodType := c.checkMethodDefinition(method)
            objType = objType.WithProperty(method.Key.Value, methodType)
        }
    }
    
    for _, prop := range body.Properties {
        if !prop.IsStatic {
            propType := c.inferPropertyType(prop)
            objType = objType.WithProperty(prop.Key.Value, propType)
        }
    }
    
    return objType
}
```

#### 2.3 Constructor Type Creation
```go
func (c *Checker) createConstructorType(ctor *parser.MethodDefinition, instanceType *types.ObjectType) *types.FunctionType {
    if ctor == nil {
        // Default constructor: () => InstanceType
        return types.NewSimpleFunction([]types.Type{}, instanceType)
    }
    
    // Parse constructor parameters and create function type
    paramTypes := c.extractParameterTypes(ctor.Value)
    return types.NewSimpleFunction(paramTypes, instanceType)
}
```

#### 2.4 New Expression Integration
```go
// Extend checker for 'new ClassName()' expressions
func (c *Checker) checkNewExpression(node *parser.NewExpression) Type {
    callee := c.checkExpression(node.Callee)
    
    if classType, ok := callee.(*types.ClassType); ok {
        // Type check constructor call
        c.checkFunctionCall(node.Arguments, classType.ConstructorType)
        return classType.InstanceType
    }
    
    // ... existing constructor function logic
}
```

### Phase 3: Compilation Strategy ⏳ **PLANNED**

**Goal**: Generate bytecode that creates constructor functions with proper prototype setup.

#### 3.1 Compilation Overview
```go
// Add to compiler/compile_class.go
func (c *Compiler) compileClassDeclaration(node *parser.ClassDeclaration) {
    // 1. Compile constructor function
    c.compileConstructor(node.Body.Constructor, node.Body.Properties)
    
    // 2. Create and setup prototype object
    c.setupPrototype(node.Body.Methods)
    
    // 3. Link constructor and prototype
    c.linkConstructorPrototype()
    
    // 4. Store constructor in environment
    c.storeConstructor(node.Name.Value)
}
```

#### 3.2 Constructor Compilation
```go
func (c *Compiler) compileConstructor(ctor *parser.MethodDefinition, props []*parser.PropertyDefinition) {
    if ctor == nil {
        // Generate default constructor
        c.compileDefaultConstructor(props)
        return
    }
    
    // Compile constructor function with property initialization
    c.compileFunctionWithPropertyInit(ctor.Value, props)
}

func (c *Compiler) compileDefaultConstructor(props []*parser.PropertyDefinition) {
    // Generate: function ClassName() { /* property initialization */ }
    c.startFunction(0, false, "constructor")
    
    // Initialize declared properties
    for _, prop := range props {
        if !prop.IsStatic && prop.Value != nil {
            c.compilePropertyInitialization(prop)
        }
    }
    
    // Implicit return this
    c.emit(OpGetThis)
    c.emit(OpReturn)
    c.endFunction()
}
```

#### 3.3 Property Initialization
```go
func (c *Compiler) compilePropertyInitialization(prop *parser.PropertyDefinition) {
    // Generate: this.propertyName = initializer
    thisReg := c.emitGetThis()
    
    if prop.Value != nil {
        valueReg := c.compileExpression(prop.Value)
        c.emit(OpSetProperty, thisReg, prop.Key.Value, valueReg)
    }
    // If no initializer, property remains undefined
}
```

#### 3.4 Prototype Setup
```go
func (c *Compiler) setupPrototype(methods []*parser.MethodDefinition) {
    // Create prototype object
    prototypeReg := c.emit(OpNewObject)
    
    // Add methods to prototype
    for _, method := range methods {
        if method.Kind != "constructor" && !method.IsStatic {
            methodReg := c.compileFunction(method.Value)
            c.emit(OpSetProperty, prototypeReg, method.Key.Value, methodReg)
        }
    }
    
    return prototypeReg
}

func (c *Compiler) linkConstructorPrototype() {
    // Set Constructor.prototype = prototypeObject
    // Set prototypeObject.constructor = Constructor
    constructorReg := c.getCurrentConstructor()
    prototypeReg := c.getCurrentPrototype()
    
    c.emit(OpSetProperty, constructorReg, "prototype", prototypeReg)
    c.emit(OpSetProperty, prototypeReg, "constructor", constructorReg)
}
```

### Phase 4: VM Runtime Support ⏳ **PLANNED**

**Goal**: Ensure VM can execute class-generated bytecode correctly.

#### 4.1 No New Opcodes Required
Since classes compile to standard constructor functions, existing opcodes handle everything:
- `OpCall` - Constructor calls
- `OpNew` - Object instantiation  
- `OpSetProperty` - Prototype setup
- `OpGetProperty` - Method/property access

#### 4.2 Constructor Function Enhancement
```go
// Ensure constructor functions work with 'new' operator
// (Already implemented in existing prototype system)
```

#### 4.3 Method Binding
```go
// Methods automatically get proper 'this' binding through prototype chain
// (Already handled by existing method call implementation)
```

## 🧪 Testing Strategy

### Phase 1-2 Tests: Basic Class Declaration and Expressions
```typescript
// Class declaration
class Person {
  name: string;
  
  constructor(name: string) {
    this.name = name;
  }
}

// Class expression (named)
const Animal = class Pet {
  species: string;
  constructor(species: string) {
    this.species = species;
  }
};

// Class expression (anonymous)
const Vehicle = class {
  wheels: number;
  constructor(wheels: number) {
    this.wheels = wheels;
  }
};

let p = new Person("Alice");
let dog = new Animal("Canis lupus");
let car = new Vehicle(4);
```

### Phase 3 Tests: Methods
```typescript
// Class with methods
class Calculator {
  value: number;
  
  constructor(initial: number = 0) {
    this.value = initial;
  }
  
  add(n: number): number {
    this.value += n;
    return this.value;
  }
  
  getValue(): number {
    return this.value;
  }
}

let calc = new Calculator(10);
calc.add(5);
console.log(calc.getValue()); // 15
```

### Phase 4 Tests: Type Checking
```typescript
// Type checking and inference
class Animal {
  species: string;
  
  constructor(species: string) {
    this.species = species;
  }
  
  getSpecies(): string {
    return this.species;
  }
}

let dog: Animal = new Animal("Canis lupus");
let species: string = dog.getSpecies(); // Type inference works
```

## 🔄 Future Enhancements (Post-Basic Implementation)

### Phase 5: Inheritance (`extends`)
```typescript
class Dog extends Animal {
  breed: string;
  
  constructor(breed: string) {
    super("Canis lupus");
    this.breed = breed;
  }
}
```

### Phase 6: Static Members
```typescript
class MathUtils {
  static PI = 3.14159;
  
  static square(n: number): number {
    return n * n;
  }
}
```

### Phase 7: Access Modifiers
```typescript
class BankAccount {
  private balance: number;
  public owner: string;
  
  constructor(owner: string, initialBalance: number) {
    this.owner = owner;
    this.balance = initialBalance;
  }
  
  public deposit(amount: number): void {
    this.balance += amount;
  }
  
  private validateTransaction(amount: number): boolean {
    return amount > 0 && amount <= this.balance;
  }
}
```

### Phase 8: Abstract Classes
```typescript
abstract class Shape {
  abstract area(): number;
  
  describe(): string {
    return `Shape with area ${this.area()}`;
  }
}
```

## 🎯 Success Criteria

### Phase 1-2: Basic Classes Complete
- ✅ Class declaration parsing works
- ✅ Constructor and method definitions recognized
- ✅ Property declarations supported
- ✅ Type checker creates proper class types

### Phase 3: Compilation Complete
- ✅ Classes compile to constructor functions
- ✅ Prototypes set up correctly with methods
- ✅ Property initialization works in constructors
- ✅ `new ClassName()` instantiation works

### Phase 4: Runtime Complete
- ✅ Class instances behave like objects
- ✅ Method calls work with proper `this` binding
- ✅ Property access and assignment work
- ✅ Type checking prevents invalid operations

## 🏁 Implementation Timeline

1. **Week 1**: Phase 1-2 (Lexer, Parser, Basic Types)
2. **Week 2**: Phase 3 (Compilation Strategy)  
3. **Week 3**: Phase 4 (Runtime Integration & Testing)
4. **Week 4**: Polish, edge cases, and comprehensive testing

## 🤝 Benefits

### For Developers
- **Modern TypeScript syntax** - Classes are fundamental to TS development
- **Type safety** - Full compile-time checking of class usage
- **Familiar patterns** - Standard OOP concepts work as expected
- **Gradual adoption** - Can mix classes with existing function-based code

### For Paserati
- **Major language milestone** - Classes are essential for serious TypeScript support
- **Library compatibility** - Many TypeScript libraries depend on classes
- **Framework support** - Enables popular frameworks that use class-based components
- **Developer productivity** - Modern OOP patterns increase development speed

By building on Paserati's solid foundation of functions, prototypes, and type checking, we can deliver a robust class implementation that feels native to both JavaScript and TypeScript developers.
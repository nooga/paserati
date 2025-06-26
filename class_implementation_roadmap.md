# TypeScript Class Implementation Roadmap

## ✅ **Currently Working (Basic Classes)**
- ✅ Basic class declarations: `class Name {}`
- ✅ Property declarations without types: `prop;`, `prop = value;`  
- ✅ Constructor methods: `constructor() {}`
- ✅ Basic instance methods: `method() {}`
- ✅ Class instantiation: `new Class()`
- ✅ Property access: `obj.prop`
- ✅ Method calls: `obj.method()`

## 🔧 **Next Priority: Core Type System Integration**

### 1. **Property Type Annotations** (HIGH PRIORITY)
**Issue**: Property declarations with type annotations fail
```typescript
name: string;     // ❌ Parser error: "expected identifier in class body"
age: number;      // ❌ Same error
```
**Root Cause**: Class property parser doesn't handle `: type` syntax
**Files to modify**: `pkg/parser/parse_class.go` - `parseProperty()` method

### 2. **Method Parameter & Return Types** (HIGH PRIORITY)
**Issue**: Method signatures with types fail completely
```typescript
getName(): string { }           // ❌ Parser error
method(param: type): type { }   // ❌ Parser error
```
**Root Cause**: Method parsing doesn't handle TypeScript parameter/return type syntax
**Files to modify**: `pkg/parser/parse_class.go` - method parsing in `parseClassBody()`

### 3. **Constructor Parameter Types** (HIGH PRIORITY)
**Issue**: Constructor with typed parameters fails
```typescript
constructor(name: string, age: number) { } // ❌ Parser error
```
**Root Cause**: Same as method parameters - type annotations not supported
**Files to modify**: Constructor parsing logic

## 🚀 **Medium Priority: Access Control & Modifiers**

### 4. **Access Modifiers** (MEDIUM PRIORITY)
**Issue**: `public`, `private`, `protected` keywords treated as properties
```typescript
private name: string;   // ❌ Parsed as property named "private"
public method() {}      // ❌ Parsed as property named "public"
```
**Current**: Actually parsing but treating modifiers as property names
**Files to modify**: `pkg/parser/parse_class.go` - add access modifier parsing

### 5. **Static Members** (MEDIUM PRIORITY)
**Issue**: `static` keyword not recognized
```typescript
static count = 0;        // ❌ "static" becomes property name
static getCount() {}     // ❌ Same issue
```
**Files to modify**: Class body parsing to handle `static` keyword

### 6. **Readonly Properties** (MEDIUM PRIORITY)
**Issue**: `readonly` keyword not supported
```typescript
readonly id: number = 42; // ❌ Parser error
```

## 🎯 **Advanced Features (Lower Priority)**

### 7. **Optional Properties** (LOW PRIORITY)
```typescript
name?: string;           // ❌ Optional syntax not supported
method(param?: type)     // ❌ Optional parameters not supported
```

### 8. **Inheritance** (LOW PRIORITY)
```typescript
class Dog extends Animal // ❌ extends keyword not supported
super.method()           // ❌ super calls not supported
```

### 9. **Getters/Setters** (LOW PRIORITY)
```typescript
get name(): string {}    // ❌ get/set keywords not supported
set name(value: string) {}
```

## 📋 **Implementation Plan**

### Phase 1: Type Annotations (Week 1)
1. ✅ AST dump utility (DONE)
2. Fix property type parsing: `name: string;`
3. Fix method return type parsing: `method(): type`
4. Fix parameter type parsing: `method(param: type)`
5. Update type checker integration

### Phase 2: Access Modifiers (Week 2)  
1. Add `public`, `private`, `protected` parsing
2. Add `static` keyword support
3. Add `readonly` modifier support
4. Update AST nodes with modifier fields

### Phase 3: Advanced Features (Future)
1. Optional properties (`?` syntax)
2. Inheritance (`extends`, `super`)  
3. Getters/setters (`get`/`set`)
4. Generic classes (`<T>`)
5. Abstract classes
6. Interface implementation

## 🎯 **Immediate Next Steps**

1. **Fix property type annotations**: Modify `parseProperty()` in `parse_class.go`
2. **Fix method type annotations**: Modify method parsing logic  
3. **Test with existing test files**: Use the FIXME test files to validate
4. **Update AST dump**: Add better support for new node types

The parser foundation is solid - we just need to extend it to handle TypeScript's type annotation syntax in class contexts.
# Super Keyword Implementation

## ECMAScript Specification Requirements

### Super Syntax Restrictions

The `super` keyword has strict syntactic constraints per ECMAScript spec:

**Valid uses:**
- `super.property` - Property access
- `super[expr]` - Computed property access
- `super(...)` - Super constructor call (only in derived class constructors)

**Invalid uses:**
- `var x = super` - Standalone expression (SyntaxError)
- `(0, super).x` - Comma operator (SyntaxError)
- `super + 1` - Binary operation (SyntaxError)

This means `super` does NOT behave like a normal identifier and requires special compilation.

### Super Property Access Semantics

Per ECMAScript §12.3.5 (Super Keyword):

1. **MakeSuperPropertyReference** creates a Reference with:
   - **base**: `env.GetSuperBase()` (the prototype of [[HomeObject]])
   - **propertyKey**: The property name
   - **thisValue**: `env.GetThisBinding()` (the **original `this`**, not the super base!)
   - **strict**: Whether in strict mode

2. **GetValue** on this Reference calls:
   ```
   base.[[Get]](propertyKey, thisValue)
   ```
   Where `thisValue` is the **original `this`**, not the super base.

3. **SetValue** on this Reference calls:
   ```
   base.[[Set]](propertyKey, value, thisValue)
   ```
   Where `thisValue` is the **original `this`**.

**Critical insight:**
- When accessing `super.prop` where `prop` is a getter, the getter is called with **original `this`** as receiver
- When assigning `super.prop = value` where `prop` is a setter, the setter is called with **original `this`** as receiver
- When assigning `super.prop = value` where `prop` is a data property, the property is set **on original `this`**, not on super base

Examples:
```javascript
class A {
  get x() { return this; }
  set y(v) { this._y = v; }
}
class B extends A {
  method() {
    // Getter called with this = B instance (NOT A.prototype)
    const val = super.x;

    // Setter called with this = B instance (NOT A.prototype)
    super.y = 10;

    // Data property set on B instance (NOT A.prototype)
    super.z = 20;

    // Method called with this = B instance (normal call semantics)
    return super.m() + 1;
  }
}
```

## Correct Implementation Strategy

### Why Dedicated Super Opcodes?

**Super is fundamentally different from normal property access:**

1. **Dual-object semantics:**
   - **Lookup** happens on `homeObject.prototype` (super base)
   - **Receiver** (for getters/setters/this-binding) is original `this`

2. **Cannot be generalized:**
   - Normal `OpGetProp` only knows about one object
   - Super needs both super base AND original `this`

3. **Performance:**
   - Single instruction vs. multi-instruction sequence
   - Clear semantics vs. complex workarounds

4. **ECMAScript spec treats it specially:**
   - Dedicated MakeSuperPropertyReference operation
   - Different from MakePropertyReference

### Required Opcodes

```
OpGetSuper Rd, NameIdx          // super.property read
OpSetSuper NameIdx, Rv          // super.property write
OpGetSuperComputed Rd, Rk       // super[expr] read
OpSetSuperComputed Rk, Rv       // super[expr] write
```

All opcodes must:
1. Get super base from `frame.homeObject.prototype`
2. Use `frame.thisValue` as receiver for getters/setters
3. Set data properties on `frame.thisValue` (for Set opcodes)
4. Walk prototype chain from super base

## Current Implementation Status

### What Already Exists (As of 2025-01-15)

**Opcodes already defined:**
- ✅ `OpGetSuper` - exists at opcode 112
- ✅ `OpSetSuper` - exists at opcode 113
- ❌ `OpGetSuperComputed` - needs to be added
- ❌ `OpSetSuperComputed` - needs to be added

**[[HomeObject]] infrastructure:**
- ✅ `FunctionObject.HomeObject` field
- ✅ `CallFrame.homeObject` field
- ✅ `OpDefineMethod` sets [[HomeObject]] for class methods
- ✅ `OpDefineMethod` sets [[HomeObject]] for object literal methods (ShorthandMethod)
- ⚠️  Computed property name methods don't get [[HomeObject]]

**Existing OpGetSuper/OpSetSuper are 90% correct:**
- ✅ Receiver handling: Uses `frame.thisValue` for getter/setter calls
- ✅ Data property assignment: Sets on `frame.thisValue` (not super base)
- ✅ Prototype chain walking: Correctly walks from super base
- ❌ **BUG:** Gets super base from `frame.thisValue.prototype` instead of `frame.homeObject.prototype`

## Implementation Plan

### Phase 1: Fix Existing Opcodes (CRITICAL)

**1. Fix OpGetSuper super base lookup:**

File: `pkg/vm/vm.go` (around line 4660)

```go
// CURRENT (WRONG):
var protoValue Value
if thisValue.Type() == TypeObject {
    obj := thisValue.AsPlainObject()
    protoValue = obj.prototype  // ❌ Wrong: using this.prototype
}

// FIXED:
var protoValue Value
homeObject := frame.homeObject
if homeObject.Type() == TypeObject {
    obj := homeObject.AsPlainObject()
    protoValue = obj.prototype  // ✅ Correct: using homeObject.prototype
}
```

**2. Fix OpSetSuper super base lookup:**

File: `pkg/vm/vm.go` (around line 4915)

Same change - use `frame.homeObject.prototype` instead of `frame.thisValue.prototype`

### Phase 2: Add Computed Property Opcodes

**3. Add OpGetSuperComputed:**

Similar to OpGetSuper but:
- Takes property key from register (instead of constant)
- Uses same super base + receiver logic

```go
case OpGetSuperComputed:
    destReg := code[ip]
    ip++
    propertyReg := code[ip]
    ip++

    homeObject := frame.homeObject
    // Get super base from homeObject.prototype
    // Get property using registers[propertyReg] as key
    // Use frame.thisValue as receiver for getters
```

**4. Add OpSetSuperComputed:**

Similar to OpSetSuper but:
- Takes property key from register
- Uses same super base + receiver logic

### Phase 3: Update Compiler

**5. Fix SuperExpression compilation:**

File: `pkg/compiler/compiler.go` (around line 852)

```go
// CURRENT (WRONG):
case *parser.SuperExpression:
    // Load super base (homeObject.prototype) per ECMAScript spec
    c.chunk.WriteOpCode(vm.OpLoadSuper, node.Token.Line)
    c.chunk.WriteByte(byte(hint))
    return hint, nil

// FIXED:
case *parser.SuperExpression:
    // Super is not a valid standalone expression
    return BadRegister, NewCompileError(node,
        "'super' keyword unexpected here - can only be used in super.property, super[expr], or super()")
```

**6. Update compileSuperMemberExpression:**

File: `pkg/compiler/compile_expression.go` (around line 2295)

```go
func (c *Compiler) compileSuperMemberExpression(node *parser.MemberExpression, hint Register, tempRegs *[]Register) (Register, errors.PaseratiError) {
    // Check if this is computed property access (super[expr])
    if computedKey, ok := node.Property.(*parser.ComputedPropertyName); ok {
        // Compile the property expression into a register
        propertyReg := c.regAlloc.Alloc()
        *tempRegs = append(*tempRegs, propertyReg)
        _, err := c.compileNode(computedKey.Expr, propertyReg)
        if err != nil {
            return BadRegister, err
        }

        // Emit OpGetSuperComputed
        c.chunk.WriteOpCode(vm.OpGetSuperComputed, node.Token.Line)
        c.chunk.WriteByte(byte(hint))
        c.chunk.WriteByte(byte(propertyReg))
        return hint, nil
    }

    // Static property access: super.property
    propertyName := c.extractPropertyName(node.Property)
    nameConstIdx := c.chunk.AddConstant(vm.String(propertyName))

    // Emit OpGetSuper
    c.chunk.WriteOpCode(vm.OpGetSuper, node.Token.Line)
    c.chunk.WriteByte(byte(hint))
    c.chunk.WriteUint16(nameConstIdx)
    return hint, nil
}
```

**7. Update assignment compilation for super:**

File: `pkg/compiler/compile_statement.go` or wherever assignment is handled

Need to detect `super.property = value` and `super[expr] = value` patterns and emit OpSetSuper/OpSetSuperComputed.

### Phase 4: Fix Computed Property Name Methods

**8. Add [[HomeObject]] for computed name methods:**

File: `pkg/compiler/compile_literal.go` (around line 720)

```go
// For computed method names, need runtime support to set [[HomeObject]]
// Option 1: Add OpDefineMethodComputed
// Option 2: Emit OpDefineMethod with runtime key-to-name conversion
// Option 3: Set [[HomeObject]] explicitly after OpSetIndex
```

This requires more investigation - may need new opcode or helper.

### Phase 5: Testing

**9. Test each fix incrementally:**

```bash
# Test basic super property access
./paserati-test262 -path ./test262 -pattern "prop-dot-cls-ref-this.js"

# Test super with getters
./paserati-test262 -path ./test262 -pattern "getter-super-prop.js"

# Test computed property names
./paserati-test262 -path ./test262 -subpath "language/computed-property-names/object/method"

# Full super suite
./paserati-test262 -path ./test262 -subpath "language/expressions/super" -suite
```

**10. Run baseline diff:**

```bash
./paserati-test262 -path ./test262 -subpath "language" -timeout 0.2s -diff baseline.txt
```

Expected outcome: Net change +13 (fix the 13 regressions we introduced)

## Summary of Required Changes

| File | Change | Priority |
|------|--------|----------|
| `pkg/vm/vm.go` | Fix OpGetSuper super base lookup | P0 - Critical |
| `pkg/vm/vm.go` | Fix OpSetSuper super base lookup | P0 - Critical |
| `pkg/vm/vm.go` | Add OpGetSuperComputed handler | P1 - High |
| `pkg/vm/vm.go` | Add OpSetSuperComputed handler | P1 - High |
| `pkg/vm/bytecode.go` | Define OpGetSuperComputed/OpSetSuperComputed | P1 - High |
| `pkg/vm/bytecode.go` | Add disassembly cases | P1 - High |
| `pkg/compiler/compiler.go` | Fix SuperExpression to error | P0 - Critical |
| `pkg/compiler/compile_expression.go` | Update compileSuperMemberExpression | P1 - High |
| `pkg/compiler/compile_statement.go` | Add super assignment handling | P1 - High |
| `pkg/compiler/compile_literal.go` | Fix computed method names | P2 - Medium |

## Expected Test262 Impact

**After Phase 1 (Fix existing opcodes):**
- Fix 13 regressions related to super property access with getters/setters
- Net change: +13

**After Phase 2 (Add computed opcodes):**
- Enable super[expr] to work correctly
- Additional tests passing

**After Phase 3 (Fix compiler):**
- Prevent invalid super usage
- Cleaner semantics

**After Phase 4 (Computed method names):**
- Full super support in all contexts

## References

- ECMAScript §12.3.5: Super Keyword
- ECMAScript §12.3.5.3: MakeSuperPropertyReference
- ECMAScript §6.2.4: GetValue (V)
- ECMAScript §6.2.4: SetValue (V, W)
- V8 blog: https://v8.dev/blog/fast-super
- Test262 tests: `test262/test/language/expressions/super/`

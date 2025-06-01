# `this` Keyword Implementation Plan

## Current State Analysis

Paserati uses a register-based VM with these calling conventions:

- Function in register `funcReg`
- Arguments in registers `funcReg+1`, `funcReg+2`, etc.
- Return value goes to `destReg` in caller's frame

**Current OpCall format:** `OpCall destReg funcReg argCount`

## High-Performance Design Strategy

Modern JS engines (V8, SpiderMonkey) treat `this` as an **implicit first parameter**. This is the most performant approach.

## Backwards-Compatible Implementation Plan

### Phase 1: Add New Opcodes (No Breaking Changes)

Keep existing `OpCall` working, add new opcodes for `this` support:

#### A. New Bytecode Instructions (bytecode.go)

```go
const (
    // ... existing opcodes ...
    OpCall       OpCode = 19 // Rx FuncReg ArgCount: Call function (UNCHANGED)
    OpCallThis   OpCode = 50 // Rx FuncReg ThisReg ArgCount: Call with explicit this
    OpCallMethod OpCode = 51 // Rx ObjReg MethodReg ArgCount: Method call (obj becomes this)
    OpLoadThis   OpCode = 52 // Rx: Load current this value into register
)
```

#### B. Enhanced VM (vm.go)

```go
case OpCallThis:
    destReg := code[ip]
    funcReg := code[ip+1]
    thisReg := code[ip+2]
    argCount := int(code[ip+3])
    ip += 4

    // Same as OpCall but with explicit this handling
    callerRegisters := registers
    callerIP := ip
    calleeVal := callerRegisters[funcReg]

    switch calleeVal.Type() {
    case TypeClosure:
        // ... setup new frame ...

        // NEW: Copy this to first parameter slot
        newFrame.registers[0] = callerRegisters[thisReg]

        // Copy args starting from register 1
        for i := 0; i < argCount; i++ {
            argRegInCaller := funcReg + 3 + byte(i) // Skip funcReg, thisReg
            newFrame.registers[1+i] = callerRegisters[argRegInCaller]
        }
        // ... rest of call setup ...

case OpCallMethod:
    destReg := code[ip]
    objReg := code[ip+1]
    methodReg := code[ip+2]
    argCount := int(code[ip+3])
    ip += 4

    // Method calls: this = object
    // Get method function from object property (already in methodReg)
    callerRegisters := registers
    methodVal := callerRegisters[methodReg]

    // Setup call with obj as this
    // ... similar to OpCallThis but this = obj ...

case OpLoadThis:
    destReg := code[ip]
    ip++

    // Load this from register 0 of current frame
    registers[destReg] = registers[0]
```

### Phase 2: Parser/AST Updates

#### A. Add ThisExpression (ast.go)

```go
type ThisExpression struct {
    BaseExpression
    Token lexer.Token // 'this' token
}

func (te *ThisExpression) expressionNode() {}
func (te *ThisExpression) TokenLiteral() string { return te.Token.Literal }
func (te *ThisExpression) String() string { return "this" }
```

#### B. Parser Registration (parser.go)

```go
// In NewParser:
p.registerPrefix(lexer.THIS, p.parseThisExpression)

func (p *Parser) parseThisExpression() Expression {
    return &ThisExpression{Token: p.curToken}
}
```

#### C. Lexer Update (lexer.go)

```go
// Add to keywords map:
"this": lexer.THIS,
```

### Phase 3: Compiler Updates

#### A. Enhanced Call Expression Compilation (compiler.go)

```go
func (c *Compiler) compileCallExpression(node *parser.CallExpression) errors.PaseratiError {
    // Detect method call vs function call
    if memberExpr, isMethodCall := node.Function.(*parser.MemberExpression); isMethodCall {
        return c.compileMethodCall(memberExpr, node.Arguments, node.Token.Line)
    } else {
        return c.compileFunctionCall(node.Function, node.Arguments, node.Token.Line)
    }
}

func (c *Compiler) compileMethodCall(memberExpr *parser.MemberExpression, args []parser.Expression, line int) errors.PaseratiError {
    // 1. Compile object (this value)
    err := c.compileNode(memberExpr.Object)
    if err != nil { return err }
    objReg := c.regAlloc.Current()

    // 2. Get method from object property
    propName := memberExpr.Property.Value
    nameConstIdx := c.chunk.AddConstant(vm.String(propName))
    methodReg := c.regAlloc.Alloc()
    c.emitGetProp(methodReg, objReg, nameConstIdx, line)

    // 3. Compile arguments in contiguous registers
    startReg := c.regAlloc.Alloc()
    for i, arg := range args {
        if i > 0 { c.regAlloc.Alloc() } // Ensure contiguous
        err = c.compileNode(arg)
        if err != nil { return err }

        expectedReg := startReg + Register(i)
        actualReg := c.regAlloc.Current()
        if actualReg != expectedReg {
            c.emitMove(expectedReg, actualReg, line)
        }
    }

    // 4. Emit method call
    resultReg := c.regAlloc.Alloc()
    c.emitCallMethod(resultReg, objReg, methodReg, len(args), line)

    return nil
}

func (c *Compiler) compileFunctionCall(funcExpr parser.Expression, args []parser.Expression, line int) errors.PaseratiError {
    // Use existing OpCall for backwards compatibility OR
    // Use new OpCallThis with undefined this

    // 1. Compile function
    err := c.compileNode(funcExpr)
    if err != nil { return err }
    funcReg := c.regAlloc.Current()

    // 2. Create undefined this
    thisReg := c.regAlloc.Alloc()
    c.emitLoadUndefined(thisReg, line)

    // 3. Compile arguments (existing logic)
    // ... existing arg compilation ...

    // 4. Emit call with this
    resultReg := c.regAlloc.Alloc()
    c.emitCallThis(resultReg, funcReg, thisReg, len(args), line)

    return nil
}

// Compile this expressions
func (c *Compiler) compileNode(node parser.Node) errors.PaseratiError {
    switch node := node.(type) {
    // ... existing cases ...

    case *parser.ThisExpression:
        destReg := c.regAlloc.Alloc()
        c.emitLoadThis(destReg, node.Token.Line)
        return nil
    }
}
```

#### B. New Emit Helpers (emit.go)

```go
func (c *Compiler) emitCallThis(dest, funcReg, thisReg Register, argCount int, line int) {
    c.emitOpCode(vm.OpCallThis, line)
    c.emitByte(byte(dest))
    c.emitByte(byte(funcReg))
    c.emitByte(byte(thisReg))
    c.emitByte(byte(argCount))
}

func (c *Compiler) emitCallMethod(dest, objReg, methodReg Register, argCount int, line int) {
    c.emitOpCode(vm.OpCallMethod, line)
    c.emitByte(byte(dest))
    c.emitByte(byte(objReg))
    c.emitByte(byte(methodReg))
    c.emitByte(byte(argCount))
}

func (c *Compiler) emitLoadThis(dest Register, line int) {
    c.emitOpCode(vm.OpLoadThis, line)
    c.emitByte(byte(dest))
}
```

### Phase 4: Checker Updates

```go
case *parser.ThisExpression:
    // Type of 'this' depends on context
    // For now, use 'any' - could be refined based on function context
    node.SetComputedType(types.Any)
```

### Phase 5: Migration Strategy

1. **Backwards Compatibility**: Keep existing `OpCall` working
2. **Gradual Migration**: New functions use `OpCallThis`/`OpCallMethod`
3. **Testing**: Both old and new calling conventions work
4. **Future**: Eventually deprecate old `OpCall`

## Implementation Steps

### Step 1: Add Lexer Support

```bash
# Add THIS token to lexer
git checkout -b feature/this-lexer
# Edit lexer.go to add THIS token
# Test lexer changes
```

### Step 2: Add AST Nodes

```bash
# Add ThisExpression to parser
# Add parsing support
# Test parser changes
```

### Step 3: Add VM Opcodes

```bash
# Add OpCallThis, OpCallMethod, OpLoadThis
# Implement VM execution
# Test with simple this examples
```

### Step 4: Add Compiler Support

```bash
# Add compilation for ThisExpression
# Add method call detection
# Test end-to-end this functionality
```

### Step 5: Add Checker Support

```bash
# Add type checking for this expressions
# Test type safety
```

## Example: Complete Method Call Flow

```typescript
let obj = {
  name: "test",
  getName: function () {
    return this.name;
  },
};
let result = obj.getName(); // this = obj
```

Compiled bytecode:

```
R0 = makeObject()                    // obj
R1 = loadConst "test"               // name value
R2 = setProp R0, "name", R1         // obj.name = "test"
R3 = makeClosure(getName)           // getName function
R4 = setProp R0, "getName", R3      // obj.getName = function
R5 = getProp R0, "getName"          // load method
R6 = callMethod R0, R5, 0           // call obj.getName(), this=R0
```

Inside `getName` function:

```
R0 = loadThis                       // this (= obj)
R1 = getProp R0, "name"             // this.name
R2 = return R1                      // return this.name
```

This approach provides:

- **Maximum performance**: `this` is just a register
- **Clean semantics**: Method calls vs function calls are distinct
- **Backwards compatibility**: Existing code continues to work
- **Modern design**: Matches high-performance JS engines

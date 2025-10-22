# Assignment Compilation Architecture

This document describes how assignments are compiled in Paserati, covering both simple assignments and destructuring patterns.

## Overview

Assignment compilation is centralized in `pkg/compiler/compile_assignment.go`. The architecture supports:
- Simple assignments (`x = value`)
- Compound assignments (`x += value`, `x *= value`, etc.)
- Destructuring assignments (array and object patterns)
- Nested destructuring patterns
- Default values in destructuring

## Entry Points

### `compileAssignmentExpression(node *parser.AssignmentExpression, hint Register)`

Main entry point for all assignment expressions. Handles three LHS (left-hand side) types:

1. **Identifier** (`x = value`)
   - Resolves variable in scope chain
   - Handles local variables, upvalues, globals, and `with` properties

2. **IndexExpression** (`arr[i] = value`)
   - Compiles object and index expressions
   - Uses `OpGetIndex` / `OpSetIndex` bytecode

3. **MemberExpression** (`obj.prop = value`)
   - Handles static properties (`obj.prop`)
   - Handles computed properties (`obj[expr]`)
   - Handles private fields (`obj.#field`)
   - Special handling for `super` property assignment

### Compound Assignment Flow

For compound operators (`+=`, `-=`, `*=`, etc.):

1. **Load current value** from LHS into temporary register
2. **Apply operation** (add, subtract, multiply, etc.)
3. **Store result** back to LHS

Example: `x += 5` becomes:
```
currentValue = GET x
newValue = currentValue + 5
SET x = newValue
```

## Destructuring Assignments

### Array Destructuring

Handled by `compileArrayDestructuringAssignment()`:

```javascript
[a, b, c] = expr
```

Desugars to:
```
temp = expr
a = temp[0]
b = temp[1]
c = temp[2]
```

#### Features:
- **Rest elements**: `[a, ...rest] = arr` uses `OpArraySlice` to get remaining elements
- **Default values**: `[a = 1, b = 2] = arr` uses conditional assignment
- **Nested patterns**: `[a, [b, c]] = arr` recursively destructures
- **Elision/holes**: `[a, , c] = arr` skips undefined positions

### Object Destructuring

Handled by `compileObjectDestructuringAssignment()`:

```javascript
{a, b, c} = expr
```

Desugars to:
```
temp = expr
a = temp.a
b = temp.b
c = temp.c
```

#### Features:
- **Property renaming**: `{a: x, b: y} = obj` assigns `obj.a` to `x`
- **Default values**: `{a = 1} = obj` uses conditional assignment
- **Nested patterns**: `{user: {name, age}} = obj` recursively destructures
- **Rest properties**: `{a, ...rest} = obj` collects remaining properties
- **Computed properties**: `{[expr]: value} = obj` evaluates expression

### Recursive Assignment

`compileRecursiveAssignment()` handles all target types:
- `Identifier` → `compileIdentifierAssignment()`
- `MemberExpression` → `compileMemberExpressionAssignment()`
- `IndexExpression` → `compileIndexExpressionAssignment()`
- `ArrayLiteral` → `compileNestedArrayDestructuring()`
- `ObjectLiteral` → `compileNestedObjectDestructuring()`

### Default Values

`compileConditionalAssignment()` implements default value logic:

```javascript
[a = defaultExpr] = arr
```

Compiles to:
```
temp = arr[0]
a = (temp !== undefined) ? temp : defaultExpr
```

Uses `OpJumpIfNotUndefined` bytecode for efficiency.

## Bytecode Operations

### Assignment Opcodes
- `OpSetLocal` - Store to local variable register
- `OpSetGlobal` - Store to global variable by index
- `OpSetFree` - Store to upvalue (closure variable)
- `OpSetProp` - Store to object property
- `OpSetIndex` - Store to array/object index
- `OpSetPrivateField` - Store to private class field
- `OpSetSuper` - Store to super property (static)
- `OpSetSuperComputed` - Store to super property (computed)

### Retrieval Opcodes
- `OpGetLocal` - Load from local variable register
- `OpGetGlobal` - Load from global variable by index
- `OpLoadFree` - Load from upvalue
- `OpGetProp` - Load from object property
- `OpGetIndex` - Load from array/object index
- `OpGetPrivateField` - Load from private class field
- `OpGetSuper` - Load from super property (static)
- `OpGetSuperComputed` - Load from super property (computed)

### Utility Opcodes
- `OpArraySlice` - Extract subarray for rest elements
- `OpJumpIfNotUndefined` - Conditional jump for default values
- `OpMove` - Copy value between registers

## Code Paths

### Simple Assignment: `x = 5`
```
Parser → AssignmentExpression
       → Left: Identifier("x")
       → Value: NumberLiteral(5)

Compiler → compileAssignmentExpression()
         → Resolve identifier scope
         → Compile RHS into register
         → Emit OpSetLocal/OpSetGlobal/OpSetFree
```

### Array Destructuring: `[a, b] = arr`
```
Parser → ArrayDestructuringAssignment
       → Elements: [Identifier("a"), Identifier("b")]
       → Value: Identifier("arr")

Compiler → compileArrayDestructuringAssignment()
         → Compile RHS into temp register
         → For each element:
           - Get temp[index]
           - Assign to target via compileRecursiveAssignment()
```

### Object Destructuring: `{x, y} = obj`
```
Parser → ObjectDestructuringAssignment
       → Properties: [{Target: Identifier("x")}, {Target: Identifier("y")}]
       → Value: Identifier("obj")

Compiler → compileObjectDestructuringAssignment()
         → Compile RHS into temp register
         → For each property:
           - Get temp.propertyName
           - Assign to target via compileRecursiveAssignment()
```

## Register Management

Assignments use temporary registers extensively:
- **Automatic cleanup** via `defer` statements
- **Register allocation** tracks all temps in `tempRegs` slice
- **Early freeing** when possible to reduce register pressure

## Scoping Rules

The compiler handles different variable scopes:

1. **Local variables**: Direct register access (fastest)
2. **Upvalues**: Closure variables via `OpLoadFree`/`OpSetFree`
3. **Global variables**: Indexed globals via `OpGetGlobal`/`OpSetGlobal`
4. **With properties**: Property access on with-object via `OpGetProp`/`OpSetProp`

Priority order (highest to lowest):
1. With properties (checked first if in with-scope)
2. Local variables in current function
3. Upvalues from outer functions
4. Global variables (fallback)

## Edge Cases

### Assignment as Expression
Assignments return their RHS value:
```javascript
x = y = 5  // Both x and y get 5, expression returns 5
```

### Compound Assignment with Getters/Setters
For `obj.prop += 5`, the getter is called once:
```
temp = obj.prop      // Calls getter
temp = temp + 5
obj.prop = temp      // Calls setter
```

### Super Property Assignment
Uses specialized opcodes that handle dual-object semantics:
- Property lookup on super base
- Receiver binding uses original `this` for setters

## Known Limitations

Current implementation does not support:
- **Destructuring with default values in expression context**: `[a = b] = arr` where `b` is complex expression may not work in all contexts
- **Pattern matching in catch clauses**: `catch ({message})` may have issues
- **Assignment patterns in for-in/for-of**: Some edge cases with `for (const {x} of arr)` may fail

## Testing Strategy

Test destructuring assignments with:
1. Simple patterns: `[a, b] = arr`, `{x, y} = obj`
2. Default values: `[a = 1] = arr`, `{x = 1} = obj`
3. Nested patterns: `[a, [b, c]] = arr`, `{x: {y, z}} = obj`
4. Rest elements: `[a, ...rest] = arr`, `{x, ...rest} = obj`
5. Mixed scenarios: Combination of above

Enable debug mode by setting `debugAssignment = true` in `compile_assignment.go`.

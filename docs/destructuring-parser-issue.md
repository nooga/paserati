# Destructuring Assignment Parser Issue

## Problem

The parser currently fails to parse destructuring assignments with default values when the pattern appears in expression context:

```javascript
// FAILS - parsed as array literal
[ a = b ] = arr

// WORKS - recognized as destructuring in declaration
const [ a = b ] = arr

// WORKS - recognized as destructuring in variable statement
let x, y;
[x, y] = arr
```

## Root Cause

**Ambiguity between array literals and destructuring patterns:**

When the parser encounters `[...]` in expression context, it doesn't know whether it's:
1. An array literal: `[a, b, c]`
2. A destructuring pattern: `[a, b] = rhs`

The parser commits to array literal parsing early and fails when it encounters `=` inside the brackets.

## Test Case

**Failing:**
```javascript
let flag1 = false, flag2 = false;
let _;
let vals = [14];
[ _ = flag1 = true, _ = flag2 = true ] = vals;  // Parse error!
```

**Error:**
```
SyntaxError: expected ',' or ']' in array literal
```

## Why It Happens

In `parseArrayLiteral()`, when we see `[`, we parse elements as expressions. But in destructuring patterns, elements can have:
- Default values: `a = expr`
- Nested patterns: `[a, b]` or `{x, y}`
- Rest elements: `...rest`

The grammar conflict:
```
ArrayLiteral: [ ElementList ]
DestructuringPattern: [ BindingElementList ]

Where BindingElement can be:
  - BindingIdentifier Initializer?  // a or a = expr
  - BindingPattern Initializer?     // [a,b] or {x,y}
```

## JavaScript Spec Solution

ECMAScript uses **cover grammars**:
1. Parse `[...]` as "CoverParenthesizedExpressionAndArrowParameterList"
2. Keep both interpretations valid during parsing
3. Refine to correct interpretation when context is known

Or use **lookahead**:
1. When seeing `[` in expression context
2. Look ahead to see if there's `] =` pattern
3. Choose array literal vs destructuring based on lookahead

## Current Workarounds

### Workaround 1: Use parentheses
```javascript
// Force expression context with parentheses
([a, b] = arr);  // Works (but still might not support defaults)
```

### Workaround 2: Use declarations
```javascript
// Use let/const/var to signal destructuring
let [a = 1, b = 2] = arr;  // Works
```

## Proposed Solutions

### Option 1: Lookahead (Simple but Limited)
```
When parseExpression() sees '[':
  1. Save parser state
  2. Scan ahead looking for '] ='
  3. If found, parse as destructuring assignment
  4. Otherwise, parse as array literal
```

**Pros**: Simple to implement
**Cons**: Doesn't handle all cases (e.g., `for ([a] of arr)`)

### Option 2: Cover Grammar (Spec-Compliant)
```
Parse [...] as ArrayLiteral/DestructuringPattern ambiguous node:
  - Allow both interpretations
  - Store enough info for both
  - Refine when '=' is seen or not
```

**Pros**: Handles all cases, spec-compliant
**Cons**: Complex, requires AST node changes

### Option 3: Reparse (Pragmatic)
```
When parseExpression() returns ArrayLiteral:
  - Check if next token is '=' (assignment)
  - If yes, convert ArrayLiteral to DestructuringPattern
  - Continue parsing as assignment
```

**Pros**: Minimal parser changes
**Cons**: Requires conversion logic, might miss edge cases

## Recommendation

**Start with Option 3 (Reparse)** because:
1. Least invasive to current parser structure
2. Can be implemented incrementally
3. Handles common cases
4. Can be refined to Option 2 later if needed

## Implementation Plan

1. **Detect assignment pattern**:
   - In `parseExpression()`, after getting an ArrayLiteral
   - Check if next token is an assignment operator (`=`, `+=`, etc.)
   - If yes and operator is `=`, this might be destructuring

2. **Convert ArrayLiteral to DestructuringPattern**:
   - Create helper: `convertArrayLiteralToPattern()`
   - Validate each element can be a valid destructuring target
   - Handle default values (elements with assignments)
   - Reject invalid patterns (e.g., literals, complex expressions)

3. **Parse as assignment**:
   - Create `ArrayDestructuringAssignment` node
   - Continue with existing destructuring compilation logic

## Test Cases to Support

```javascript
// Basic with defaults
[a = 1, b = 2] = arr

// Chained assignments in defaults
[a = x = 1, b = y = 2] = arr

// Nested patterns with defaults
[[a = 1], b = 2] = arr

// Rest with defaults (invalid)
[a = 1, ...rest] = arr  // default before rest

// Complex defaults
[a = foo(), b = bar()] = arr
```

## Status

- [x] Identified root cause
- [x] Created smoke tests
- [x] Documented issue
- [ ] Implement solution
- [ ] Test with Test262 suite

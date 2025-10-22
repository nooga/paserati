# Destructuring Assignment Fix - Production Implementation Plan

## Root Cause Analysis

The parser already implements a cover grammar for array destructuring patterns (lines 5287-5303 in `parser.go`), BUT there's a precedence bug that prevents chained assignments in default values.

### Current Code (Lines 5289-5295)

```go
if p.peekTokenIs(lexer.ASSIGN) {
    p.nextToken() // Consume '='
    p.nextToken() // Move to default value expression
    defaultExpr := p.parseExpression(ASSIGNMENT)  // BUG: Wrong precedence!
    if defaultExpr == nil {
        return nil
    }
    // Create AssignmentExpression to represent the default
    elem = &AssignmentExpression{
        Token:    p.curToken,
        Operator: "=",
        Left:     elem,
        Value:    defaultExpr,
    }
}
```

### The Problem

When parsing `[a = b = c]`:

1. Parse `a` at ASSIGNMENT precedence → gets identifier `a`
2. See `=` at peek → enter default value handling
3. Consume `=`, advance to `b`
4. **Parse default at ASSIGNMENT precedence** → parses only `b`, stops before `= c`
5. Returns with `b` as the default, but `= c` is unparsed
6. Next iteration expects `,` or `]` but finds `=` → ERROR

### Why ASSIGNMENT Precedence is Wrong

Pratt parser precedences (from low to high):
```
LOWEST         - lowest, parses everything
COMMA          - stops at comma
ARG_SEPARATOR  - stops at comma, allows assignment
ASSIGNMENT     - stops at assignment operators (=, +=, etc.)
TERNARY        - higher...
```

When you call `parseExpression(ASSIGNMENT)`, it parses expressions with precedence >= ASSIGNMENT, which EXCLUDES assignment operators (because they have ASSIGNMENT precedence themselves).

To parse the full `b = c` expression, we need to use `ARG_SEPARATOR` precedence (which includes assignments but excludes commas).

## The Fix

### Change 1: Fix Default Expression Parsing (Line 5292)

```go
// OLD:
defaultExpr := p.parseExpression(ASSIGNMENT)

// NEW:
defaultExpr := p.parseExpression(ARG_SEPARATOR)
```

This allows the default expression to include assignment operators, so `b = c` parses completely.

### Change 2: Same Fix for Element Parsing (Line 5282)

Actually, line 5282 is correct as-is:
```go
elem := p.parseExpression(ASSIGNMENT)
```

This is intentional! We want to parse `a` but NOT `= b` at this point, because we handle `= b` specially in the cover grammar section.

## Validation

### Test Case 1: Simple Default
```javascript
[a = 1] = arr
```
- Parse `a` at ASSIGNMENT → gets `a`
- See `=`, parse `1` at ARG_SEPARATOR → gets `1`
- Element: `AssignmentExpression(a, 1)` ✓

### Test Case 2: Chained Assignment Default
```javascript
[a = b = 1] = arr
```
- Parse `a` at ASSIGNMENT → gets `a`
- See `=`, parse `b = 1` at ARG_SEPARATOR → gets `AssignmentExpression(b, 1)`
- Element: `AssignmentExpression(a, AssignmentExpression(b, 1))` ✓

### Test Case 3: Multiple Elements with Defaults
```javascript
[a = x = 1, b = y = 2] = arr
```
- First element: `AssignmentExpression(a, AssignmentExpression(x, 1))`
- Comma separates
- Second element: `AssignmentExpression(b, AssignmentExpression(y, 2))` ✓

## Implementation

### File: `pkg/parser/parser.go`

**Location**: Line 5292 in `parseArrayLiteral()`

**Change**:
```diff
  if p.peekTokenIs(lexer.ASSIGN) {
      p.nextToken() // Consume '='
      p.nextToken() // Move to default value expression
-     defaultExpr := p.parseExpression(ASSIGNMENT)
+     defaultExpr := p.parseExpression(ARG_SEPARATOR)
      if defaultExpr == nil {
          return nil
      }
```

**Rationale**: `ARG_SEPARATOR` precedence allows assignment expressions but stops at commas, which is exactly what we need for parsing default values in destructuring patterns.

## Testing

### Smoke Tests
- `tests/scripts/destructuring_assignment_basic.ts` - Already passes ✓
- `tests/scripts/destructuring_assignment_defaults.ts` - Already passes ✓
- `tests/scripts/destructuring_assignment_chained_defaults.ts` - Will pass after fix

### Test262 Impact
- `language/expressions/assignment/dstr` subsuite (187 failing tests)
- Many should start passing with this one-line fix

### Regression Risk
**LOW** - This change only affects default value parsing in array literals. The change makes it MORE permissive (allows more valid syntax), so existing code will continue to work.

## Production Quality Checklist

- [x] Root cause identified with precision
- [x] Minimal change (one line)
- [x] No new code paths (uses existing ARG_SEPARATOR precedence)
- [x] Clear rationale documented
- [x] Test cases prepared
- [x] Regression risk assessed (low)
- [ ] Fix implemented
- [ ] Smoke tests passing
- [ ] Test262 improvement validated

## Alternative Considered: Change Element Parsing

We could change line 5282 to:
```go
elem := p.parseExpression(ARG_SEPARATOR)
```

But this would be WRONG because it would parse `a = b` as a single element (an AssignmentExpression), and we wouldn't enter the default value handling block. The current approach with cover grammar is correct.

## ECMAScript Spec Alignment

This fix aligns with ECMAScript spec grammar:

```
ArrayBindingPattern :
    [ Elision? BindingRestElement? ]
    [ BindingElementList ]
    [ BindingElementList , Elision? BindingRestElement? ]

BindingElement :
    SingleNameBinding
    BindingPattern Initializer?

Initializer :
    = AssignmentExpression
```

The spec says the initializer is an `AssignmentExpression`, which means it CAN contain assignment operators. Our current code uses `ASSIGNMENT` precedence which excludes them. Using `ARG_SEPARATOR` allows them, matching the spec.

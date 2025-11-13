# Root Cause: Infinite Recursion in Generator Destructuring

## The Bug

When a generator object is used as the source for array destructuring, and `isDirectCall = false`, infinite recursion occurs:

```
executeGeneratorPrologue → vm.run() → [destructuring creates generator] →
executeGeneratorPrologue → vm.run() → [destructuring creates generator] → ...
```

## Reproduction

```typescript
var iter = function*() { }();  // Create generator object
const [[] = function() { return iter; }()] = [];  // Destructure from it
// Stack overflow!
```

## Why This Happens

1. Destructuring pattern `[[] = init()]` evaluates the initializer
2. Initializer returns a generator object `iter`
3. Destructuring tries to iterate over `iter` to get the nested `[]`
4. Iterating calls the generator
5. Generator creation triggers `executeGeneratorPrologue`
6. With `isDirectCall = false`, this runs in nested `vm.run()`
7. The prologue contains the SAME destructuring code
8. Loop continues forever

## Why It Didn't Happen Before

With `isDirectCall = true`:
- Exception unwinding stopped at the prologue frame boundary
- Frame was explicitly cleaned up by `executeGeneratorPrologue`
- The nested `vm.run()` returned cleanly
- Some mechanism prevented the infinite recursion (need to investigate what)

## The Real Question

**Why are these tests passing in the baseline (+211 improvement)?**

If this causes infinite recursion, how did we get +211 passing tests? Either:
1. The recursion only happens in specific cases
2. There's a guard somewhere preventing it
3. The tests that passed are different from these 50 regressions

## Next Steps

1. Check if there's recursion depth limiting
2. Understand what changed between baseline and these regressions
3. The fix for the 50 regressions must NOT break the +211 improvement

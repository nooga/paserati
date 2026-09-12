// expect: true
// paserati#443: a named function expression's self-reference (the binding
// that lets `function fact() { ... fact() ... }` call itself by name) is
// stored in a dedicated register that the VM initializes with the closure
// itself right before the function body starts running (see
// call.go prepareCall / NameBindingRegister). The compiler used to allocate
// that register LAST - after compiling default-parameter and destructuring-
// parameter code, which allocates and frees its own temporary registers for
// things like the "is this param undefined?" check. When the function is
// nested inside another function (so it can't fall back to resolving its
// own name as a global/outer-scope variable), the register allocator could
// hand the name-binding register the exact number a temp register had just
// been freed from. That temp register's bytecode - default-parameter
// evaluation, which runs at function entry, ahead of the rest of the body -
// then clobbered the self-reference with its own temporary value before the
// recursive call ever executed, throwing "TypeError: object is not a
// function".
//
// Precisely narrowed (see the issue): needs (a) the function nested inside
// another function (so its own name isn't resolvable as an outer/global
// variable) and (b) a destructured parameter with a *whole-pattern* default
// (`{...} = {}`), not just an inner-field default or no default at all. This
// is exactly the shape used throughout ajv's compiled validators
// (`function validate0(data, {instancePath="", ...}={}) {...}`), so it broke
// every recursive ajv schema validation.
//
// The fix: reserve and pin the name-binding register right after parameters
// are defined, before any default-value/destructuring temp registers are
// allocated, so it can never be recycled out from under the closure.

const checks: boolean[] = [];

// --- Case 1: the issue's own minimal repro, function-declaration nesting ---
function makeFact(): (n: number) => number {
  return function fact(n: number, { step = 1 } = {}): number {
    if (n <= 0) return 0;
    return step + fact(n - 1, { step });
  };
}
checks.push(makeFact()(3) === 3);

// --- Case 2: nested inside an IIFE, matching what `new Function(src)` (a
// bare source string compiled as `return (function() { <src> });`) produces
// under the hood ---
const fact2 = (function () {
  return function fact(n: number, { step = 2 } = {}): number {
    if (n <= 0) return 0;
    return step + fact(n - 1, { step });
  };
})();
checks.push(fact2(3) === 6);

// --- Case 3: an actual `new Function(...)`-compiled recursive validator,
// the real-world ajv shape from the issue ---
const makeFn = new Function(`
  return function fact(n, {step = 1} = {}) {
    if (n <= 0) return 0;
    return step + fact(n - 1, {step});
  };
`);
const fact3 = makeFn();
checks.push(fact3(4) === 4);

// --- Case 4: sanity check that a parameter actually named the same as the
// function still shadows the self-binding (the register is reserved eagerly
// now, but must still be released when shadowed) ---
function makeShadowed(): (fact: number) => string {
  return function fact(fact: number): string {
    return typeof fact;
  };
}
checks.push(makeShadowed()(5) === "number");

checks.every((c) => c);

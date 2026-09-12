// expect: true
// Found while investigating paserati#443. A rest parameter's array is not
// looked up by name at call time: the VM hard-codes its destination as
// register index `calleeFunc.Arity` (the count of named, non-rest
// parameters) - see prepareCall (call.go), several inlined fast paths in
// vm.go, and executeAsyncFunctionBody (async.go). That is only the correct
// register number if the compiler handed out parameter registers gaplessly,
// in order, starting at 0 - true immediately after the named-parameter
// loop, since nothing has been freed yet.
//
// The compiler used to allocate the rest parameter's register *after*
// compiling any default-parameter values (`param = expr`), which allocate
// and free their own temporary registers (an "is this param undefined?"
// comparison, the default-value expression itself). Whenever a default
// value preceded the rest parameter, one of those freed temporaries could
// get recycled into the rest parameter's Alloc() call instead of it getting
// a fresh, sequential register - landing the rest parameter one register
// past where the VM actually writes its array. Reading the rest parameter
// by name then read whatever stale value (or live temporary) happened to be
// sitting in the wrong register instead of the real array - no destructuring
// required, a single plain default value is enough:
// `function f(n, step = 1, ...rest) { return rest; }` returned garbage
// instead of the rest array.
//
// This affected every function-literal compile path that has its own copy
// of "define params, then defaults, then rest param": plain function
// declarations/expressions and object/class methods
// (compileFunctionLiteralWithOptions), arrow functions
// (compileArrowFunctionLiteral/compileArrowFunctionWithName), and object
// literal shorthand methods (compileShorthandMethod).
//
// Fix: reserve (and pin) the rest parameter's register immediately after
// the named-parameter loop, before any default-value temporaries can be
// allocated - the same discipline #443's fix applied to the named function
// expression self-reference register.

const checks: boolean[] = [];

// --- Case 1: the plainest possible repro - no destructuring at all ---
function plainFn(n: number, step: number = 1, ...rest: number[]): number[] {
  return rest;
}
const r1 = plainFn(1, 2, 9, 10);
checks.push(r1.length === 2 && r1[0] === 9 && r1[1] === 10);

// --- Case 2: empty rest array (no extra args past the defaulted param) ---
checks.push(Array.isArray(plainFn(1)) && plainFn(1).length === 0);

// --- Case 3: arrow function ---
const arrowFn = (n: number, step: number = 1, ...rest: number[]): boolean =>
  Array.isArray(rest) && rest.length === 2;
checks.push(arrowFn(1, 2, 9, 10));

// --- Case 4: object literal shorthand method ---
const obj = {
  method(n: number, step: number = 1, ...rest: number[]): boolean {
    return Array.isArray(rest) && rest.length === 2;
  },
};
checks.push(obj.method(1, 2, 9, 10));

// --- Case 5: class method ---
class C {
  method(n: number, step: number = 1, ...rest: number[]): boolean {
    return Array.isArray(rest) && rest.length === 2;
  }
}
checks.push(new C().method(1, 2, 9, 10));

// --- Case 6: destructured whole-pattern-default parameter before the rest
// parameter (the exact combination from the original investigation) ---
function withDestructuredDefault(
  n: number,
  { step = 1 } = {},
  ...rest: number[]
): boolean {
  return Array.isArray(rest) && rest.length === 2;
}
checks.push(withDestructuredDefault(1, {}, 9, 10));
checks.push(Array.isArray(withDestructuredDefault2(1)));

function withDestructuredDefault2(
  n: number,
  { step = 1 } = {},
  ...rest: number[]
): number[] {
  return rest;
}

// --- Case 7: a destructured (array-pattern) rest parameter after a default
// value, to make sure the fix covers rest patterns too, not just simple
// rest identifiers ---
function destructuredRest(
  n: number,
  step: number = 1,
  ...[a, b]: number[]
): boolean {
  return a === 9 && b === 10;
}
checks.push(destructuredRest(1, 2, 9, 10));

// --- Case 8: generator function ---
function* gen(n: number, step: number = 1, ...rest: number[]) {
  yield rest.length;
}
checks.push([...gen(1, 2, 9, 10, 11)][0] === 3);

checks.every((c) => c);

// expect: true
// Found while investigating paserati#443: a named function expression's
// self-reference (the binding that lets `function fact() { ... fact() ... }`
// call itself by name) is written into a dedicated register by
// call.go's prepareCall, right before the callee's bytecode starts running
// (see FunctionObject.NameBindingRegister). Async functions, however, don't
// go through prepareCall at all - executeAsyncFunctionBody (async.go)
// builds their very first frame by hand (so it can hand control back to the
// caller instead of switching to it, wrapping the result in a Promise), and
// that hand-built setup never applied this same initialization step.
//
// So a named async function expression nested inside another function -
// meaning its own name can't fall back to resolving as an outer/global
// variable, the way a top-level `const f = async function fact() {...}`
// does - read its own self-binding register as whatever this frame slot's
// zeroed default was, and calling it threw
// "TypeError: undefined is not a function".
//
// Fix: executeAsyncFunctionBody now mirrors prepareCall's
// NameBindingRegister initialization right after it finishes setting up
// parameter/rest-parameter registers.

const checks: boolean[] = [];

// --- Case 1: nested async NFE recursion (the actual repro) ---
function makeFact(): (n: number) => Promise<number> {
  return async function fact(n: number): Promise<number> {
    if (n <= 0) return 0;
    return 1 + (await fact(n - 1));
  };
}

// --- Case 2: same shape, but with a destructured whole-pattern-default
// parameter too (the exact #443 shape, just async) ---
function makeFact2(): (n: number) => Promise<number> {
  return async function fact(
    n: number,
    { step = 1 } = {}
  ): Promise<number> {
    if (n <= 0) return 0;
    return step + (await fact(n - 1, { step }));
  };
}

// --- Case 3: sanity check that a top-level (non-nested) async NFE, which
// worked even before this fix (it falls back to resolving its name as the
// outer `const` binding), still works ---
const fact3 = async function fact(n: number): Promise<number> {
  if (n <= 0) return 0;
  return 1 + (await fact(n - 1));
};

async function main(): Promise<boolean> {
  checks.push((await makeFact()(4)) === 4);
  checks.push((await makeFact2()(3)) === 3);
  checks.push((await fact3(5)) === 5);
  return checks.every((c) => c);
}

await main();

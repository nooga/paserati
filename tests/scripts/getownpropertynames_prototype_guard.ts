// expect: true
// Object.getOwnPropertyNames's TypeFunction and TypeClosure cases both
// unconditionally appended a synthesized "prototype" own key at the end
// whenever "prototype" wasn't already a real, explicit own property in the
// function's Properties table - regardless of whether the function was
// actually constructible:
//
//   Object.getOwnPropertyNames(() => {});           // before: ["length","name","prototype"] - Node: ["length","name"]
//   async function asy() {}
//   Object.getOwnPropertyNames(asy);                // before: ["length","name","prototype"] - Node: ["length","name"]
//
// Per spec, a function has an own "prototype" property only when it's
// actually constructible - an ordinary function, a generator function, and
// an async generator function all have one; an arrow function and a plain
// (non-generator) async function do not (verified against Node - see the
// full matrix in the checks below).
//
// Fixed with a one-line guard: vm.VM.IsConstructor(obj) already implements
// exactly this rule for TypeFunction/TypeClosure
// (!IsArrowFunction && !(IsAsync && !IsGenerator)) - reused instead of
// duplicated. The OTHER "prototype" branch (when "prototype" is already a
// real, explicit own property - e.g. after a prior `.prototype` access
// lazily created it, or user code explicitly assigned one) is untouched:
// an explicitly-created property is always listed regardless of
// constructibility.
//
// Reflect.ownKeys delegates to this same function for these two kinds
// (task_06547fb2's fix), so the identical bug affected it too - covered
// here as well, not just Object.getOwnPropertyNames.

const checks: boolean[] = [];

// --- 1. Arrow function: no "prototype" (the bug's main repro). ---
{
  const arrow = () => {};
  const keys = Object.getOwnPropertyNames(arrow);
  checks.push(keys.length === 2 && keys[0] === "length" && keys[1] === "name");
}

// --- 2. Plain (non-generator) async function: no "prototype" either. ---
{
  async function asy() {}
  const keys = Object.getOwnPropertyNames(asy);
  checks.push(keys.length === 2 && keys[0] === "length" && keys[1] === "name");
}

// --- 3. Generator function: HAS "prototype" - must not regress (a naive
// "just remove the unconditional append" fix would have broken this).
// vm.VM.IsConstructor(obj) - the predicate this fix reuses - answers
// `true` for a generator function, which happens to be exactly the
// has-"prototype" rule this check needs, but is NOT the same question as
// "is this actually usable with `new`": `new (function*(){})()` throws
// per spec, and pkg/builtins/reflect_init.go's own separate, differently-
// named isConstructor(v) helper (used by Reflect.construct to decide
// whether to throw) correctly returns false for a generator. This fix
// depends on vm.IsConstructor's non-spec-constructibility answer staying
// exactly as it is for generators/async generators - if that helper is
// ever made spec-correct for its ACTUAL callers (e.g. OpValidateSuperclass,
// which likely wrongly allows `class X extends function*(){}` today - see
// the follow-up chip), this guard and check 3/4 here will need to switch
// to a distinct has-"prototype" predicate instead. ---
{
  function* gen() {}
  const keys = Object.getOwnPropertyNames(gen);
  checks.push(keys.length === 3 && keys[0] === "length" && keys[1] === "name" && keys[2] === "prototype");
}

// --- 4. Async generator function: HAS "prototype" too - the other half of
// the same regression risk as check 3. ---
{
  async function* asyncGen() {}
  const keys = Object.getOwnPropertyNames(asyncGen);
  checks.push(keys.length === 3 && keys[0] === "length" && keys[1] === "name" && keys[2] === "prototype");
}

// --- 5. A plain function declaration: HAS "prototype" - the ordinary,
// most common case, must not regress. ---
{
  function fn(a: number, b: number) {
    return a + b;
  }
  const keys = Object.getOwnPropertyNames(fn);
  checks.push(keys.length === 3 && keys[0] === "length" && keys[1] === "name" && keys[2] === "prototype");
}

// --- 6. A class (compiles to TypeClosure, same code path as check 5's
// TypeFunction sibling through different surface syntax): HAS "prototype". ---
{
  class C {}
  const keys = Object.getOwnPropertyNames(C);
  checks.push(keys.length === 3 && keys[0] === "length" && keys[1] === "name" && keys[2] === "prototype");
}

// --- 7. Reflect.ownKeys on an arrow function agrees with
// Object.getOwnPropertyNames (it delegates to the same fixed function per
// task_06547fb2) - no "prototype" here either. ---
{
  const arrow2 = () => {};
  const keys = Reflect.ownKeys(arrow2 as any);
  checks.push(keys.length === 2 && keys[0] === "length" && keys[1] === "name");
}

// --- 8. Reflect.ownKeys on a plain async function agrees too. ---
{
  async function asy2() {}
  const keys = Reflect.ownKeys(asy2 as any);
  checks.push(keys.length === 2 && keys[0] === "length" && keys[1] === "name");
}

checks.every((c) => c === true);

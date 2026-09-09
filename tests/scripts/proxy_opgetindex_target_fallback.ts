// expect: true
// OpGetIndex's Proxy handling (pkg/vm/vm.go, `case OpGetIndex:`, backing
// bracket-notation `proxy[key]`) had its OWN "no get trap, fallback to
// target" switch, separate from opGetProp's (dot notation, `proxy.prop`,
// already fixed for the same shape of bug two commits back on this
// branch):
//
//   targetBase := proxy.target
//   switch targetBase.Type() {
//   case TypeArray:
//       ... (numeric fast path, string/symbol index delegates to opGetProp)
//   case TypeObject, TypeDictObject:
//       ... delegates to opGetProp for the key ...
//   default:
//       registers[destReg] = Undefined
//   }
//
// Every proxy.target kind besides TypeArray/TypeObject/TypeDictObject -
// TypeArguments, TypeMap, TypeSet, TypePromise, TypeRegExp, TypeGenerator,
// TypeBoundFunction, TypeNativeFunction, TypeNativeFunctionWithProps, a
// further-nested TypeProxy, ... - fell to that bare `default:` and
// silently read `undefined` for every bracket-accessed key, even though
// the identical key read via dot notation (opGetProp's own fallback,
// already correct) worked fine:
//
//   function f(a, b) {
//     const p = new Proxy(arguments, {});
//     return [p.length, p[0], p[1]]; // was [2, undefined, undefined]
//   }
//   f(10, 20); // now [2, 10, 20], matching Node
//
// Fixed by widening the `default:` case to delegate to
// getPropertyWithReceiver (pkg/vm/vm_init.go) instead of doing nothing -
// the same helper vm.GetProperty and opGetProp's own fallback are already
// built on, and it already handles every one of these kinds completely,
// INCLUDING TypeArguments's numeric-index case (which opGetProp itself
// does NOT handle - opGetProp's TypeArguments branch only ever resolves
// "length"/"callee"/named overflow properties, never a plain numeric
// index like "0"; that resolution lives separately in OpGetIndex's own
// direct, non-Proxy TypeArguments case via vm.argumentsGet - delegating
// this fallback to opGetProp instead of getPropertyWithReceiver, as an
// earlier version of this fix did, would have looked plausible and
// fixed every OTHER kind while leaving TypeArguments's numeric index
// broken). TypeArray keeps its own direct handling (a hand-rolled dense-
// element fast path for the numeric case), unchanged - only the
// `default:` arm changed.
//
// checks 1-2: the task's own repro (numeric index, "length" as a
// bracket key) and a RegExp `.lastIndex` bracket read - two kinds that
// used to fall to `default:`.
// check 3: nested trap-less Proxy target via bracket notation (the
// OpGetIndex counterpart to the earlier OpObjectSpread/opGetProp fix for
// the same shape of bug via dot notation).
// check 4: TypeArray target still works via bracket notation (the
// numeric fast path this fix did NOT touch, confirming no regression).
// check 5: a TypeDictObject handler (a TS enum at runtime) must not
// panic for the extended default case.

const checks: boolean[] = [];

// --- 1. Arguments via bracket notation: numeric index and "length" as
// an explicit bracket key. ---
{
  function f(a: number, b: number) {
    const p: any = new Proxy(arguments, {});
    return [p[0], p[1], p["length"]];
  }
  const [a0, a1, alen] = f(10, 20);
  checks.push(a0 === 10 && a1 === 20 && alen === 2);
}

// --- 2. RegExp lastIndex via bracket notation through a no-trap
// proxy. ---
{
  const re: any = /a/g;
  re.lastIndex = 3;
  const pr: any = new Proxy(re, {});
  checks.push(pr["lastIndex"] === 3);
}

// --- 3. Nested trap-less Proxy target via bracket notation. ---
{
  const inner: any = { c: 3 };
  const p1: any = new Proxy(inner, {});
  const p2: any = new Proxy(p1, {});
  const key = "c";
  checks.push(p2[key] === 3);
}

// --- 4. TypeArray target still works via bracket notation (the
// existing numeric fast path and string-key array delegation, both
// left untouched by this fix). ---
{
  const arr: any = [1, 2, 3];
  const p: any = new Proxy(arr, {});
  checks.push(p[0] === 1 && p[1] === 2 && p["length"] === 3);
}

// --- 5. A TypeDictObject handler (a TS enum at runtime) must not panic
// for the extended default case. ---
{
  enum DictHandler5 {
    A,
    B,
  }
  const re2: any = /b/;
  const pr2: any = new Proxy(re2, DictHandler5 as any);
  checks.push(pr2["lastIndex"] === 0);
}

checks.every((c) => c === true);

// expect: true
// Two independent gaps found while verifying PR #339 (native functions'
// custom own properties), both in pkg/builtins/object_init.go, both
// instances of the same "exotic VM kind missing from a per-type switch"
// bug class that's recurred throughout this stack (PRs #329-#339):
//
//   1. objectGetOwnPropertyDescriptorsWithVM had no `case
//      vm.TypeBoundFunction` at all - Object.getOwnPropertyDescriptors(
//      boundFn) returned {} entirely, missing even "name"/"length", which
//      every other callable kind's branch in this same switch synthesizes
//      explicitly:
//        function fn(a, b) {}
//        const bound = fn.bind(null);
//        Object.getOwnPropertyDescriptors(bound); // before: {} - Node: {length: {...}, name: {...}}
//      Fixed with a new case - but unlike TypeFunction/TypeNativeFunction/
//      TypeNativeFunctionWithProps/TypeClosure above it (which all
//      synthesize "name"/"length" specially, since those aren't real
//      table entries for them), a bound function's "name" ("bound " +
//      original) and "length" ARE real own properties written directly
//      into bf.Properties at bind time (pkg/vm/property_helpers.go) - so
//      a plain OwnPropertyNames()/OwnSymbolKeys() walk already includes
//      them, no special synthesis needed.
//
//   2. Object.getOwnPropertySymbols had no case for TypeNativeFunction,
//      TypeNativeFunctionWithProps, or TypeBoundFunction at all (only
//      TypeObject/TypeFunction/TypeClosure) - checked all three against
//      Node per the task's own instruction, all three were broken:
//        const nf = Array.prototype.pop;
//        const sym = Symbol("k");
//        Object.defineProperty(nf, sym, { value: 2, configurable: true });
//        Object.getOwnPropertySymbols(nf).length; // before: 0 - Node: 1
//      even though Object.getOwnPropertyDescriptor(nf, sym) already
//      correctly reported the property existing. Fixed with one shared
//      branch for all three kinds, using the generic OwnPropertiesTable
//      helper (pkg/vm/properties_table.go) - the same one
//      objectGetOwnPropertyDescriptorsWithVM and objectKeysWithVM already
//      use for the identical purpose (PR #339).
//
// Found while verifying, NOT fixed here (a real but larger, separate
// bug, flagged as a follow-up): Reflect.ownKeys throws a TypeError
// outright on ANY callable target (its object-gate never checks
// IsCallable(), unlike Reflect.get/has/etc. in the same file), and even
// past that gate has no case for any callable kind at all - so
// Reflect.ownKeys(nf) disagrees with the now-fixed
// Object.getOwnPropertySymbols(nf) above. Confirmed via:
//   Reflect.ownKeys(Array.prototype.slice); // before: throws TypeError - Node: ["length", "name", ...]
// Not a one-line fix like the two above - it needs real per-kind string-
// key synthesis (name/length/prototype per callable kind), mirroring
// objectGetOwnPropertyDescriptorsWithVM's own stringKeys collection
// logic, which is a distinctly larger scope than this task's two gaps.
//
// Each assertion below uses its own function/instance (bind()/defineProperty
// on a fresh function per check), plus distinct global constructors
// (Number/Boolean) for the two NativeFunctionWithProps checks - these are
// real shared intrinsics with no reset between assertions in the same
// test file, matching the established convention (see
// nativefunction_get_and_enumerate.ts).

const checks: boolean[] = [];

// --- 1. BoundFunction: name/length now present with the correct
// attributes (writable: false, enumerable: false, configurable: true -
// real own properties, not synthesized). ---
{
  function fn1(a: number, b: number) {}
  const bound1: any = fn1.bind(null);
  const descs: any = Object.getOwnPropertyDescriptors(bound1);
  checks.push(
    !!descs.length &&
      descs.length.value === 2 &&
      descs.length.writable === false &&
      descs.length.enumerable === false &&
      descs.length.configurable === true
  );
  checks.push(
    !!descs.name &&
      descs.name.value === "bound fn1" &&
      descs.name.writable === false &&
      descs.name.enumerable === false &&
      descs.name.configurable === true
  );
}

// --- 2. BoundFunction: a custom string property and a custom symbol
// property both show up in getOwnPropertyDescriptors alongside name/length. ---
{
  function fn2() {}
  const bound2: any = fn2.bind(null);
  Object.defineProperty(bound2, "vis2", { value: 1, enumerable: true, configurable: true });
  const sym2 = Symbol("k2");
  Object.defineProperty(bound2, sym2, { value: 2, configurable: true });
  const descs: any = Object.getOwnPropertyDescriptors(bound2);
  checks.push(!!descs.vis2 && descs.vis2.value === 1);
  checks.push(!!descs[sym2] && descs[sym2].value === 2);
}

// --- 3. BoundFunction with no custom properties: getOwnPropertyDescriptors
// reports exactly name and length, nothing extra or missing. ---
{
  function fn3() {}
  const bound3: any = fn3.bind(null);
  checks.push(Object.keys(Object.getOwnPropertyDescriptors(bound3)).length === 2);
}

// --- 4. Object.getOwnPropertySymbols on a plain native function
// (Array.prototype.*-style, TypeNativeFunction). ---
{
  const nf4: any = Array.prototype.shift;
  const sym4 = Symbol("k4");
  Object.defineProperty(nf4, sym4, { value: "v4", configurable: true });
  const syms = Object.getOwnPropertySymbols(nf4);
  checks.push(syms.length === 1 && syms[0] === sym4);
}

// --- 5. Object.getOwnPropertySymbols on a native function with props
// (a global constructor, TypeNativeFunctionWithProps). ---
{
  const sym5 = Symbol("k5");
  Object.defineProperty(Number, sym5, { value: "v5", configurable: true });
  const syms = Object.getOwnPropertySymbols(Number);
  checks.push(syms.includes(sym5));
}

// --- 6. Object.getOwnPropertySymbols on a BoundFunction, set via
// bracket-notation assignment rather than defineProperty. ---
{
  function fn6() {}
  const bound6: any = fn6.bind(null);
  const sym6 = Symbol("k6");
  bound6[sym6] = "v6";
  const syms = Object.getOwnPropertySymbols(bound6);
  checks.push(syms.length === 1 && syms[0] === sym6);
}

// --- 7. Regression guard: a native function with no custom symbol
// properties still reports an empty array, not a stale leftover. ---
{
  const nf7: any = Array.prototype.pop;
  checks.push(Object.getOwnPropertySymbols(nf7).length === 0);
}

// --- 8. Object.getOwnPropertySymbols on a second, distinct
// NativeFunctionWithProps constructor (Array, not Number/Boolean used
// above) - comparing a before/after count rather than an absolute one,
// since this is a real shared intrinsic no other check here touches
// directly. ---
{
  const before = Object.getOwnPropertySymbols(Array).length;
  const sym8 = Symbol("k8");
  Object.defineProperty(Array, sym8, { value: 1, configurable: true });
  const after = Object.getOwnPropertySymbols(Array);
  checks.push(after.length === before + 1 && after.includes(sym8));
}

checks.every((c) => c === true);

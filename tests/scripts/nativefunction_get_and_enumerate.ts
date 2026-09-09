// expect: true
// Following the fix to Object.defineProperty (and OpIn/getOwnPropertyDescriptor)
// for plain TypeNativeFunction values (defineproperty_native_function.ts),
// several more places still didn't see a native function's custom own
// properties. All three are fixed here:
//
//   1. opGetPropSymbol (pkg/vm/op_getprop.go) had no `case TypeNativeFunction`
//      at all - reading any symbol-keyed own property via `nf[sym]` (or
//      Reflect.get, though that has its own separate, much larger gap -
//      see below) returned undefined even though
//      Object.getOwnPropertyDescriptor already showed it existed.
//
//      Fixing this surfaced an asymmetry a first pass introduced: the
//      sibling string-key fix below (opGetProp's TypeNativeFunction block)
//      invokes an own accessor's getter, so this symbol-key case invokes
//      it too - otherwise `nf.custom` would call the getter while `nf[sym]`
//      would not, on the very same value. TypeFunction/TypeClosure/
//      TypeBoundFunction/TypeNativeFunctionWithProps's symbol-key cases
//      still do NOT invoke an own accessor's getter (none of the four
//      ever have) - that remains a real, separate, still-open gap.
//
//   2. opGetProp's existing TypeNativeFunction block (string keys) only
//      ever called Properties.GetOwn, which never detects or invokes an
//      accessor's getter - PlainObject.DefineAccessorProperty appends a
//      placeholder Undefined properties slot for an accessor field, so
//      GetOwn(name) returns (Undefined, true) for one instead of
//      (_, false). This silently made `nf.custom` read as undefined
//      instead of calling the getter, even though the very same
//      accessor's *setter* already ran correctly via `nf.custom = v`.
//
//   3. Object.keys, Object.getOwnPropertyDescriptors, and OpGetOwnKeys
//      (for-in, pkg/vm/vm.go) all skipped a native function's own
//      enumerable custom properties entirely - their switches never had a
//      TypeNativeFunction case. TypeNativeFunctionWithProps (e.g. `Boolean`,
//      `Number`) had the identical gap in the same switches, found and
//      fixed alongside it since the case bodies already used the generic
//      OwnPropertiesTable/`nfp.Properties` machinery and needed no new code,
//      only the additional case label.
//
// Each assertion below uses its own Array.prototype method (or, for the
// NativeFunctionWithProps checks, a distinct global constructor) - these
// are real shared intrinsics with no reset between assertions in the same
// test file, so reusing one across assertions would let one mutation leak
// into another's expectations. The three NativeFunctionWithProps checks
// (6, 8, 13) deliberately use three different constructors (Boolean,
// Number, String) for the same reason - consolidating them onto one
// constructor would silently couple those three checks together.
//
// While fixing (3), found the TypeNativeFunctionWithProps branch of
// Object.getOwnPropertyDescriptors also dropped symbol keys entirely
// (only the new TypeNativeFunction branch collected them) - fixed
// alongside it, same one-line fix on the identical *PlainObject side
// table (check 13 below).
//
// Found but NOT fixed here (flagged as separate follow-ups):
//   - Reflect.get is almost entirely non-functional beyond TypeObject/
//     TypeDictObject/TypeArray, and unconditionally stringifies its key
//     even for a Symbol - a large, pre-existing, unrelated gap.
//   - Object.getOwnPropertyDescriptors has no TypeBoundFunction case at
//     all (returns {} - missing even name/length).
//   - Object.getOwnPropertySymbols has no TypeNativeFunction case (misses
//     a symbol property that getOwnPropertyDescriptor already reports).

const checks: boolean[] = [];

// --- 1. Symbol-keyed data property read back via bracket notation ---
{
  const nf: any = Array.prototype.shift;
  const sym = Symbol("k1");
  Object.defineProperty(nf, sym, { value: 2, enumerable: true, configurable: true });
  checks.push(nf[sym] === 2);
}

// --- 2. Symbol-keyed data property set via bracket notation, read back ---
{
  const nf: any = Array.prototype.pop;
  const sym = Symbol("k2");
  nf[sym] = 5;
  checks.push(nf[sym] === 5);
}

// --- 3. String-keyed accessor: getter invoked via direct property access ---
{
  const nf: any = Array.prototype.unshift;
  let backing = 0;
  Object.defineProperty(nf, "custom", {
    get() { return backing; },
    set(v: number) { backing = v; },
    configurable: true,
  });
  nf.custom = 100;
  checks.push(backing === 100);
  checks.push(nf.custom === 100);
}

// --- 4. Symbol-keyed accessor: getter invoked via bracket access. This is
// the case that would have been silently inconsistent with #3 above (same
// kind, same commit) had the accessor check been added only on the
// string-key path. ---
{
  const nf: any = Array.prototype.slice;
  let backing = 0;
  const sym = Symbol("k4");
  Object.defineProperty(nf, sym, {
    get() { return backing; },
    set(v: number) { backing = v; },
    configurable: true,
  });
  nf[sym] = 200;
  checks.push(backing === 200);
  checks.push(nf[sym] === 200);
}

// --- 5. Object.keys sees an enumerable custom property (string key) ---
{
  const nf: any = Array.prototype.concat;
  Object.defineProperty(nf, "vis", { value: 1, enumerable: true, configurable: true });
  checks.push(JSON.stringify(Object.keys(nf)) === JSON.stringify(["vis"]));
}

// --- 6. Object.keys on TypeNativeFunctionWithProps (a native constructor) ---
{
  Object.defineProperty(Boolean, "visCtor", { value: 1, enumerable: true, configurable: true });
  checks.push(Object.keys(Boolean).includes("visCtor"));
}

// --- 7. for-in enumerates the same enumerable custom property ---
{
  const nf: any = Array.prototype.indexOf;
  Object.defineProperty(nf, "vis2", { value: 1, enumerable: true, configurable: true });
  const seen: string[] = [];
  for (const k in nf) seen.push(k);
  checks.push(JSON.stringify(seen) === JSON.stringify(["vis2"]));
}

// --- 8. for-in on TypeNativeFunctionWithProps ---
{
  Object.defineProperty(Number, "visCtor2", { value: 1, enumerable: true, configurable: true });
  const seen: string[] = [];
  for (const k in Number) seen.push(k);
  checks.push(seen.includes("visCtor2"));
}

// --- 9. Object.getOwnPropertyDescriptors reports the same custom property ---
{
  const nf: any = Array.prototype.lastIndexOf;
  Object.defineProperty(nf, "vis3", { value: 1, enumerable: true, configurable: true });
  const descs: any = Object.getOwnPropertyDescriptors(nf);
  checks.push(!!descs.vis3 && descs.vis3.value === 1);
}

// --- 10. A non-enumerable custom property is correctly excluded from
// Object.keys/for-in but still present in getOwnPropertyDescriptors ---
{
  const nf: any = Array.prototype.includes;
  Object.defineProperty(nf, "hidden", { value: 1, enumerable: false, configurable: true });
  checks.push(!Object.keys(nf).includes("hidden"));
  let seenHidden = false;
  for (const k in nf) if (k === "hidden") seenHidden = true;
  checks.push(!seenHidden);
  checks.push((Object.getOwnPropertyDescriptors(nf) as any).hidden !== undefined);
}

// --- 11. name/length are still correctly excluded from Object.keys/for-in
// (non-enumerable synthesized intrinsics, not side-table entries) ---
{
  const nf: any = Array.prototype.join;
  checks.push(!Object.keys(nf).includes("name"));
  checks.push(!Object.keys(nf).includes("length"));
}

// --- 12. Repeated string-key accessor read at the same call site: opGetProp's
// TypeNativeFunction accessor check runs before the normal inline-cache path
// used for plain objects, so a monomorphic repeated read must still call the
// getter every time, not just once. ---
{
  const nf: any = Array.prototype.reverse;
  let n = 0;
  Object.defineProperty(nf, "counter", {
    get() { return ++n; },
    configurable: true,
  });
  const seenVals: number[] = [];
  for (let i = 0; i < 3; i++) seenVals.push(nf.counter);
  checks.push(JSON.stringify(seenVals) === JSON.stringify([1, 2, 3]));
}

// --- 13. Object.getOwnPropertyDescriptors reports a symbol-keyed property
// on TypeNativeFunctionWithProps too (found while fixing (3)'s
// TypeNativeFunction sibling - the WithProps branch collected string keys
// but never symbol keys, from the same *PlainObject side table). ---
{
  const sym = Symbol("k13");
  Object.defineProperty(String, sym, { value: 9, enumerable: true, configurable: true });
  const descs: any = Object.getOwnPropertyDescriptors(String);
  checks.push(!!descs[sym] && descs[sym].value === 9);
}

checks.every((c) => c === true);

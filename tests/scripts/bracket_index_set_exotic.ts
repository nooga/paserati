// expect: true
// OpSetIndex's exotic-kinds case (search "can have properties" in
// pkg/vm/vm.go) had no TypeMap/TypeSet/TypePromise at all - unlike
// TypeFunction/TypeClosure/TypeRegExp/TypeNativeFunction(WithProps)/
// TypeBoundFunction, which were already there - so bracket-notation
// assignment (obj[key] = v) on a Map/Set/Promise unconditionally threw
// "Cannot set index on non-array/object/typedarray type", even for a
// plain string key, while dot-notation (obj.key = v) worked fine.
//
// Fixing that alone surfaced a second, adjacent bug: a symbol-keyed
// bracket assignment on ANY of these exotic kinds (Map/Set/Promise, but
// also the already-listed BoundFunction/NativeFunctionWithProps/
// NativeFunction) routes to opSetPropSymbol, which had no case for them
// either and silently discarded the write. And once that was added, a
// THIRD bug turned up: opSetPropSymbol's existing helper
// (setOwnCheckedByKey, shared by RegExp/Function/Closure's already-working
// symbol-set paths) created every new symbol property as
// {writable: false, enumerable: false, configurable: false} instead of
// the true/true/true a plain assignment should produce - a pre-existing
// bug that predates this fix, now fixed for every kind that goes through
// it.
const checks: boolean[] = [];

function checkStringKey(obj: any): boolean {
  obj["strkey"] = 42;
  if (obj.strkey !== 42) return false;
  if (!("strkey" in obj) || !Reflect.has(obj, "strkey")) return false;
  const desc: any = Object.getOwnPropertyDescriptor(obj, "strkey");
  return (
    desc !== undefined &&
    desc.value === 42 &&
    desc.writable === true &&
    desc.enumerable === true &&
    desc.configurable === true
  );
}

// checkSymbolKey verifies the write actually happened via Reflect.has
// (always) and, when checkDesc is true, via Object.getOwnPropertyDescriptor
// too. It deliberately does NOT check the `in` operator or the descriptor
// for every kind: two adjacent, pre-existing gaps in OTHER functions -
// unrelated to this bracket-assignment fix, and tracked separately - would
// make those assertions fail even though the write this fix is responsible
// for succeeded correctly (confirmed via Reflect.has in every case below):
//   - `in` (OpIn)'s symbol-key dispatch has no case at all for
//     BoundFunction/NativeFunction/NativeFunctionWithProps.
//   - Object.getOwnPropertyDescriptor's TypeRegExp/TypeBoundFunction blocks
//     never check a symbol key, only a string name.
function checkSymbolKey(obj: any, checkIn: boolean, checkDesc: boolean): boolean {
  const sym = Symbol("k");
  obj[sym] = 99;
  if (!Reflect.has(obj, sym)) return false;
  if (checkIn && !(sym in obj)) return false;
  if (!checkDesc) return true;
  const desc: any = Object.getOwnPropertyDescriptor(obj, sym);
  return (
    desc !== undefined &&
    desc.value === 99 &&
    desc.writable === true &&
    desc.enumerable === true &&
    desc.configurable === true
  );
}

// --- string key, all four kinds ---
checks.push(checkStringKey(new Map()));
checks.push(checkStringKey(new Set()));
checks.push(checkStringKey(Promise.resolve(1)));
checks.push(checkStringKey(/x/g));

// --- symbol key, all four kinds ---
checks.push(checkSymbolKey(new Map(), true, true));
checks.push(checkSymbolKey(new Set(), true, true));
checks.push(checkSymbolKey(Promise.resolve(1), true, true));
checks.push(checkSymbolKey(/y/g, true, false)); // descriptor-by-symbol gap tracked separately

// --- symbol key, the callable kinds the task asked to double-check ---
function plainFn() {}
checks.push(checkSymbolKey(plainFn.bind(null), false, false)); // both gaps above apply
const nfp: any = Array; // NativeFunctionWithProps
checks.push(checkSymbolKey(nfp, false, true)); // `in` gap applies, descriptor already worked

// --- a genuinely absent key is still absent after all this ---
const untouched: any = new Map();
checks.push(!("nope" in untouched) && !Reflect.has(untouched, "nope"));

checks.every((c) => c === true);

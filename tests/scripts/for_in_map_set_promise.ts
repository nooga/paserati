// expect: true
// OpGetOwnKeys (pkg/vm/vm.go) drives for-in enumeration by switching on the
// object's type - it had cases for TypeObject, TypeDictObject, TypeArray,
// TypeArguments, TypeFunction, TypeClosure, TypeBoundFunction, and (as of a
// prior fix) TypeRegExp, but none at all for TypeMap/TypeSet/TypePromise.
// `for (k in map/set/promise)` always came back with nothing, even after a
// plain assignment onto the value.
//
// Fixing the enumeration alone was not enough to make for-in over a
// Promise agree with Node: for-in's compiled bytecode re-verifies each
// enumerated key via OpIn's HasProperty-style check before yielding it in
// the loop body (see TypeBoundFunction's comment in OpIn, same file, for
// the identical mechanism) - and OpIn's TypePromise case never checked its
// own side table at all, only Promise.prototype, so every key
// OpGetOwnKeys had just found for a Promise was silently dropped again.
// Fixed both together: OpGetOwnKeys gained a TypeMap/TypeSet/TypePromise
// case, and OpIn's TypePromise case (both the string-key and symbol-key
// switches) now checks the same side table Reflect.has already did.
const checks: boolean[] = [];

function forInKeys(obj: any): string[] {
  const keys: string[] = [];
  for (const k in obj) {
    keys.push(k);
  }
  return keys;
}

// --- Map: a custom enumerable own property is enumerated; an empty one
// enumerates nothing; "size" (an accessor on Map.prototype, not an own
// property) and an inherited method (get) never appear ---
const m: any = new Map();
m.custom = 42;
checks.push(forInKeys(m).join(",") === "custom");
checks.push(forInKeys(new Map()).length === 0);
checks.push(forInKeys(m).indexOf("size") === -1);
checks.push(forInKeys(m).indexOf("get") === -1);

// --- Set: same shape ---
const s: any = new Set();
s.custom = 1;
checks.push(forInKeys(s).join(",") === "custom");
checks.push(forInKeys(new Set()).length === 0);
checks.push(forInKeys(s).indexOf("size") === -1);
checks.push(forInKeys(s).indexOf("add") === -1);

// --- Promise: same shape, plus this is the one that also needed the
// OpIn fix (see file comment above) - without it this enumerated empty
// despite OpGetOwnKeys finding "custom" ---
const p: any = Promise.resolve(1);
p.custom = 2;
checks.push(forInKeys(p).join(",") === "custom");
checks.push(forInKeys(Promise.resolve(1)).length === 0);
checks.push(forInKeys(p).indexOf("then") === -1);

// --- `in` and Reflect.has on a Promise's own property now agree, the
// concrete symptom of the OpIn gap this fix closed alongside for-in ---
const p2: any = Promise.resolve(3);
p2.viaAssign = 1;
checks.push(("viaAssign" in p2) && Reflect.has(p2, "viaAssign"));

checks.every((c) => c === true);

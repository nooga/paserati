// expect: true
// OpGetOwnKeys (pkg/vm/vm.go) drives for-in enumeration by switching on the
// object's type - it had no `case TypeRegExp:` at all, so `for (k in regex)`
// always came back with nothing, even after `regex.x = 1`, while the
// identical TypeFunction/TypeClosure/TypeBoundFunction cases already
// enumerated their own side table's own enumerable keys correctly.
const checks: boolean[] = [];

function forInKeys(obj: any): string[] {
  const keys: string[] = [];
  for (const k in obj) {
    keys.push(k);
  }
  return keys;
}

// --- a custom enumerable own property assigned directly onto the
// instance is enumerated ---
const r: any = /x/g;
r.custom = 42;
checks.push(forInKeys(r).join(",") === "custom");

// --- a RegExp with no custom properties enumerates nothing ---
const r2: any = /y/;
checks.push(forInKeys(r2).length === 0);

// --- "lastIndex" (own, but {enumerable: false} - it's a real Go field on
// RegExpObject, not a side-table entry) never appears ---
const r3: any = /z/g;
checks.push(forInKeys(r3).indexOf("lastIndex") === -1);

// --- inherited RegExp.prototype methods (test/exec/...) are all
// non-enumerable, so none of them appear either ---
checks.push(forInKeys(r3).indexOf("test") === -1);
checks.push(forInKeys(r3).indexOf("exec") === -1);

// --- a non-enumerable custom property (via Object.defineProperty) is
// correctly excluded, while an enumerable one alongside it still shows ---
const r4: any = /w/g;
Object.defineProperty(r4, "hidden", {
  value: 1,
  enumerable: false,
  configurable: true,
  writable: true,
});
r4.visible = 2;
checks.push(forInKeys(r4).join(",") === "visible");

// --- a symbol-keyed own property never appears (for-in is string-keys
// only, per spec) ---
const r5: any = /v/g;
r5[Symbol.iterator] = function () {};
r5.named = 1;
checks.push(forInKeys(r5).join(",") === "named");

checks.every((c) => c === true);

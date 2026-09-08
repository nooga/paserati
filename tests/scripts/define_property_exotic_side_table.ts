// expect: true
// Object.defineProperty/Reflect.defineProperty on a RegExp/Map/Set/Promise
// that has never had a property defined on it (its side table -
// OwnPropertiesTable, pkg/vm/properties_table.go - is still nil) used to
// silently do nothing while still returning the object as if it had
// succeeded: definePropertyWithVM's write-path had no branch at all for
// these four kinds (a NativeFunctionWithProps/Function/Closure-only chain),
// so it just fell through to `return obj, nil`. Object.keys and
// Object.getOwnPropertyDescriptor had the matching gap for Map/Set/Promise
// (RegExp's read side already worked via a dedicated lastIndex+side-table
// block). Plain assignment (`r.custom = 42`) was never affected - it goes
// through EnsureOwnPropertiesTable elsewhere in the property-set path.
const checks: boolean[] = [];

function roundTrips(obj: any, useReflect: boolean, checkIn: boolean): boolean {
  const definer = useReflect
    ? (o: any, k: string, d: any) => Reflect.defineProperty(o, k, d)
    : (o: any, k: string, d: any) =>
        Object.defineProperty(o, k, d) !== undefined;
  const ok = definer(obj, "custom", {
    value: 42,
    writable: true,
    enumerable: true,
    configurable: true,
  });
  if (!ok) return false;
  const desc: any = Object.getOwnPropertyDescriptor(obj, "custom");
  if (!desc || desc.value !== 42 || !desc.writable || !desc.enumerable || !desc.configurable) {
    return false;
  }
  if (!Reflect.has(obj, "custom")) return false;
  // `in` on a Promise has its own, separate pre-existing gap (never
  // checks the Promise's own side table at all, reproduces with plain
  // assignment too - unrelated to this fix, tracked separately) - skip it
  // for that one kind so this test stays about defineProperty, not that.
  if (checkIn && !("custom" in obj)) return false;
  if (Object.keys(obj).indexOf("custom") === -1) return false;
  return true;
}

// --- Object.defineProperty, virgin side table, all four kinds ---
checks.push(roundTrips(/x/g, false, true));
checks.push(roundTrips(new Map(), false, true));
checks.push(roundTrips(new Set(), false, true));
checks.push(roundTrips(Promise.resolve(1), false, false));

// --- Reflect.defineProperty, virgin side table, all four kinds ---
checks.push(roundTrips(/y/, true, true));
checks.push(roundTrips(new Map(), true, true));
checks.push(roundTrips(new Set(), true, true));
checks.push(roundTrips(Promise.resolve(2), true, false));

// --- redefining with a partial descriptor preserves the omitted value,
// not the zero Value{} DefineOwnProperty would otherwise write ---
const m: any = new Map();
Object.defineProperty(m, "foo", {
  value: 10,
  writable: true,
  enumerable: true,
  configurable: true,
});
Object.defineProperty(m, "foo", { enumerable: false });
const fooDesc: any = Object.getOwnPropertyDescriptor(m, "foo");
checks.push(fooDesc.value === 10 && fooDesc.enumerable === false);

// --- a symbol key round-trips the same way ---
const s: any = new Set();
const sym = Symbol("k");
Object.defineProperty(s, sym, { value: 7, enumerable: true, configurable: true, writable: true });
checks.push(
  Object.getOwnPropertyDescriptor(s, sym).value === 7 &&
    sym in s &&
    Reflect.has(s, sym)
);

// --- extensibility is still enforced for a brand-new property (the side
// table gets allocated by the write path now, but Object.preventExtensions
// already allocates its own via EnsureOwnPropertiesTable, so the check at
// definePropertyWithVM's extensibility guard still sees it) ---
const m2: any = new Map();
Object.preventExtensions(m2);
let threw = false;
try {
  Object.defineProperty(m2, "nope", { value: 1, configurable: true });
} catch (e: any) {
  threw = e instanceof TypeError;
}
checks.push(threw);

// --- redefining an EXISTING property still works after preventExtensions ---
const m3: any = new Map();
m3.already = 1;
Object.preventExtensions(m3);
Object.defineProperty(m3, "already", { value: 2 });
checks.push(m3.already === 2);

// --- Reflect.has finds a plain own property assigned onto a Promise
// (unrelated to defineProperty itself, but worth pinning here since the
// roundTrips helper above skips `in` for Promise entirely). `in` used to
// disagree with Reflect.has here - it never checked a Promise's own side
// table at all - but that was a separate bug in the `in` operator itself
// (OpIn, pkg/vm/vm.go), not this defineProperty fix, so it's fixed in a
// sibling PR (#332) instead of asserted on in this file: an assertion
// here would flip depending on merge order between the two PRs. ---
const pIn: any = Promise.resolve(3);
pIn.viaAssign = 1;
checks.push(Reflect.has(pIn, "viaAssign"));

// --- RegExp's "lastIndex" is excluded from the generic side-table write
// (it's a real Go field, not a table entry) - defineProperty on it stays
// the same pre-existing no-op it already was, not a new divergent shadow
// copy that would make r.lastIndex and the descriptor disagree. ---
const r: any = /z/g;
Object.defineProperty(r, "lastIndex", { value: 5 });
const liDesc: any = Object.getOwnPropertyDescriptor(r, "lastIndex");
checks.push(r.lastIndex === liDesc.value);

checks.every((c) => c === true);

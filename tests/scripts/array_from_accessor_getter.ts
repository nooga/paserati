// expect: true
// Array.from(arr) must read each source index through ordinary [[Get]],
// exactly like the generic Array.prototype methods (forEach/map/filter/...,
// via arrayLikeGet in pkg/builtins/array_generic.go) - so an own accessor
// property (installed via Object.defineProperty, get/set instead of a
// plain value) must have its getter invoked.
//
// Array.from's own array-branch fast path (pkg/builtins/array_init.go)
// used to read every element with `sourceArray.Get(i)` - the raw backing
// slot directly. DefineAccessorProperty never writes an accessor's value
// into the backing elements slice at all (it only ever touches
// getters/setters/propertyDesc), so an accessor index's raw slot is stale
// (or a leftover Hole, for an index that was never a data property to
// begin with) instead of the value ordinary [[Get]] must produce. This is
// the same gap the array spread fix (extractSpreadArguments, pkg/vm/vm.go)
// closed for `[...arr]`.
//
// Fixing this by routing element reads through arrayLikeGet (rather than
// duplicating just the accessor check) also fixes a second, related gap
// as a side effect: a genuinely absent index (a real hole - via
// `new Array(n)`, `delete`, or a `.length` bump past the dense storage)
// now falls through to the prototype chain via ordinary [[Get]] too,
// exactly like Node - not just "read Undefined off the raw backing slot"
// like the old fast path did for every OOB/hole index unconditionally.
const checks: boolean[] = [];

// Basic case: a getter on an in-bounds index of a dense array.
const a: any[] = [1, 2, 3];
let getCalls = 0;
Object.defineProperty(a, "1", {
  get() {
    getCalls++;
    return 42;
  },
  enumerable: true,
  configurable: true,
});

checks.push(a[1] === 42); // sanity: plain property access already worked

const fromResult = Array.from(a);
checks.push(JSON.stringify(fromResult) === "[1,42,3]");
checks.push(getCalls === 2); // once for the a[1] sanity check above, once for Array.from

// Array.from's mapFn is applied to the getter's return value, not the raw
// backing slot.
checks.push(JSON.stringify(Array.from(a, (x: any) => x * 2)) === "[2,84,6]");

// Multiple accessors, ascending order - the getter for each index must be
// called at most once, and results must land at the right positions.
const multi: any[] = [10, 20, 30, 40];
const calls: number[] = [];
Object.defineProperty(multi, "0", {
  get() {
    calls.push(0);
    return "first";
  },
  enumerable: true,
  configurable: true,
});
Object.defineProperty(multi, "3", {
  get() {
    calls.push(3);
    return "last";
  },
  enumerable: true,
  configurable: true,
});
checks.push(JSON.stringify(Array.from(multi)) === '["first",20,30,"last"]');
checks.push(calls.join(",") === "0,3");

// An accessor with no getter (setter-only) reads as `undefined`, per
// ordinary [[Get]] on an accessor property whose [[Get]] is absent - not
// the stale/raw backing slot.
const setterOnly: any[] = [1, 2, 3];
Object.defineProperty(setterOnly, "1", {
  set(_v: any) {},
  enumerable: true,
  configurable: true,
});
checks.push(JSON.stringify(Array.from(setterOnly)) === "[1,null,3]");

// A getter that throws must propagate out of Array.from as a real
// exception, not be swallowed or read as some placeholder value.
const throwing: any[] = [1, 2, 3];
Object.defineProperty(throwing, "0", {
  get() {
    throw new Error("boom");
  },
  enumerable: true,
  configurable: true,
});
let threw = false;
try {
  Array.from(throwing);
} catch (e: any) {
  threw = e.message === "boom";
}
checks.push(threw);

// Same, but with a mapFn also present - the throw must still escape
// rather than the mapFn call site swallowing it or yielding a partial
// array.
let threwWithMapFn = false;
try {
  Array.from(throwing, (x: any) => x);
} catch (e: any) {
  threwWithMapFn = e.message === "boom";
}
checks.push(threwWithMapFn);

// Routing element reads through arrayLikeGet (instead of the raw
// sourceArray.Get(i)) also means a genuinely absent index (a real hole,
// not just an accessor) now falls through to the prototype chain via
// ordinary [[Get]], exactly like Node - not just "read Undefined off the
// raw backing slot" like the old fast path did. Confirm this net-new
// correctness gain (not merely "unbroken") explicitly, since it isn't a
// no-op alongside the accessor fix.
const protoDesc = Object.getOwnPropertyDescriptor(Array.prototype, "1");
(Array.prototype as any)[1] = "from-proto";
const sparse = new Array(3);
checks.push(JSON.stringify(Array.from(sparse)) === '[null,"from-proto",null]');

const deletedHole: any[] = [10, 20, 30];
delete deletedHole[1];
checks.push(JSON.stringify(Array.from(deletedHole)) === '[10,"from-proto",30]');
if (protoDesc) {
  Object.defineProperty(Array.prototype, "1", protoDesc);
} else {
  delete (Array.prototype as any)[1];
}

// The common case (no accessors at all) must remain exactly as correct as
// before - this is the pre-existing fast path, unaffected by the
// accessor-aware read added alongside it.
checks.push(JSON.stringify(Array.from([1, 2, 3])) === "[1,2,3]");

// A getter is arbitrary script - it can shrink the array's own backing
// storage (`.length = 0`, `.pop()`, ...) out from under Array.from while
// it's still in the middle of reading the source. This must never panic
// the VM - it's not required to match a real iterator's "re-read .length
// every step" semantics exactly (out of scope here, see the file-level
// comment), just not crash.
const shrinking: any[] = [1, 2, 3];
Object.defineProperty(shrinking, "0", {
  get() {
    shrinking.length = 0;
    return "g";
  },
  configurable: true,
});
let shrinkPanicked = false;
try {
  Array.from(shrinking);
} catch (e) {
  shrinkPanicked = true;
}
checks.push(!shrinkPanicked);

checks.every((c) => c === true);

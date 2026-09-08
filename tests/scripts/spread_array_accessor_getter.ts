// expect: true
// A real array spread (`[...arr]`, `f(...arr)`) reads the source through its
// default iterator, which reads each index via ordinary [[Get]] - so an own
// accessor property (installed via Object.defineProperty, get/set instead
// of a plain value) must have its getter invoked, exactly like plain
// property access (`arr[i]`) already does.
//
// extractSpreadArguments's TypeArray fast path (pkg/vm/vm.go) used to
// bypass this entirely: `copy(args, arrayObj.elements)` reads the raw
// backing slot directly, and DefineAccessorProperty never writes an
// accessor's value into `elements` at all (it only ever touches
// getters/setters/propertyDesc - see pkg/vm/array_props.go) - so the
// spread result carried whatever was in that slot before the accessor was
// installed (or a Hole, for an index that was never a data property to
// begin with) instead of calling the getter.
//
// This does NOT cover holes/length handling (a separate, pre-existing gap
// in the same fast path - see the array-spread hole-propagation fix) -
// only the accessor case.
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

const spread = [...a];
checks.push(JSON.stringify(spread) === "[1,42,3]");
checks.push(getCalls === 2); // once for the a[1] sanity check above, once for the spread

// Spread into a function call goes through the same fast path.
function collect(...args: any[]) {
  return args;
}
checks.push(JSON.stringify(collect(...a)) === "[1,42,3]");

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
checks.push(JSON.stringify([...multi]) === '["first",20,30,"last"]');
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
checks.push(JSON.stringify([...setterOnly]) === "[1,null,3]");

// A getter that throws must propagate out of the spread as a real
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
  [...throwing];
} catch (e: any) {
  threw = e.message === "boom";
}
checks.push(threw);

// The common case (no accessors at all) must remain exactly as fast/
// correct as before - this is the pre-existing fast path, unaffected by
// the accessor-aware slow path added alongside it.
const plain = [1, 2, 3];
checks.push(JSON.stringify([...plain]) === "[1,2,3]");

// A getter is arbitrary script - it can shrink the array's own backing
// storage (`.length = 0`, `.pop()`, ...) out from under the spread that's
// still in the middle of reading it. This must never panic the VM (the
// slow path indexes the raw backing slice for every non-accessor read,
// so a shrink invalidates indices a naive snapshot-length loop would
// still try to read) - it's not required to match a real iterator's
// "re-read .length every step" semantics (that's out of scope here, see
// the file-level comment), just not crash.
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
  JSON.stringify([...shrinking]);
} catch (e) {
  shrinkPanicked = true;
}
checks.push(!shrinkPanicked);

checks.every((c) => c === true);

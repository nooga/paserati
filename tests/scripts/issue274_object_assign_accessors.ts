// expect: true
// paserati#274: Object.assign neither read through a source getter nor wrote
// through a target setter - it treated every property as a plain data slot
// regardless of the source's or target's actual accessor properties.
//
// Per spec, Object.assign copies each own enumerable source property via
// [[Get]] (which for an accessor property means calling the getter) and
// writes it onto the target via [[Set]] (which must invoke a setter if the
// target already defines one there).
const checks: boolean[] = [];

// --- Read side: source getter must be invoked, not skipped. ---
const source: any = {};
Object.defineProperty(source, "types", {
  get() {
    return { real: true };
  },
  enumerable: true,
  configurable: true,
});
const target: any = Object.assign({}, source);
checks.push(typeof target.types === "object");
checks.push(JSON.stringify(target.types) === '{"real":true}');
// The property genuinely lands on target - it's the *value* that was wrong.
checks.push("types" in target);
checks.push(JSON.stringify(Object.keys(target)) === '["types"]');

// A getter that returns different values on each call - Object.assign must
// call it exactly once per assign, at copy time (like real [[Get]]), not
// defer/re-invoke it.
let getterCalls = 0;
const source2: any = {};
Object.defineProperty(source2, "n", {
  get() {
    getterCalls++;
    return getterCalls;
  },
  enumerable: true,
  configurable: true,
});
const target2: any = Object.assign({}, source2);
checks.push(target2.n === 1);
checks.push(getterCalls === 1);

// --- Write side: target setter must be invoked, not clobbered. ---
const target3: any = {};
let captured: any;
Object.defineProperty(target3, "x", {
  set(v: any) {
    captured = v;
  },
  enumerable: true,
  configurable: true,
});
Object.assign(target3, { x: 42 });
checks.push(captured === 42);
// The setter itself decided to drop the value on the floor - Object.assign
// must not additionally create a shadow data property with the raw value.
checks.push(target3.x === undefined);

// --- Both sides together: a getter's value flows through a setter. ---
const src: any = {};
Object.defineProperty(src, "v", {
  get() {
    return "hello";
  },
  enumerable: true,
  configurable: true,
});
const dst: any = {};
let dstCaptured: any;
Object.defineProperty(dst, "v", {
  set(v: any) {
    dstCaptured = v;
  },
  enumerable: true,
  configurable: true,
});
Object.assign(dst, src);
checks.push(dstCaptured === "hello");

// Object spread already handled this correctly - keep it passing alongside
// the Object.assign fix (regression guard, not a new assertion about spread).
const spreadTarget: any = { ...source };
checks.push(typeof spreadTarget.types === "object");

// A throwing getter must propagate as a catchable exception, not get
// swallowed or crash the VM - the getter is invoked via vmInstance.Call from
// native Go code, outside any bytecode frame's normal unwinding path.
let threw = false;
const throwSrc: any = {};
Object.defineProperty(throwSrc, "boom", {
  get() {
    throw new Error("from getter");
  },
  enumerable: true,
  configurable: true,
});
try {
  Object.assign({}, throwSrc);
} catch (e: any) {
  threw = e.message === "from getter";
}
checks.push(threw);

// Same for a throwing setter.
let setterThrew = false;
const throwDst: any = {};
Object.defineProperty(throwDst, "boom", {
  set(_v: any) {
    throw new Error("from setter");
  },
  enumerable: true,
  configurable: true,
});
try {
  Object.assign(throwDst, { boom: 1 });
} catch (e: any) {
  setterThrew = e.message === "from setter";
}
checks.push(setterThrew);

checks.every((c) => c === true);

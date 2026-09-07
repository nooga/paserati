// expect: true
// paserati#302: redefining an existing accessor property with a *partial*
// descriptor - one that specifies enumerable/configurable/writable but omits
// get/set/value - silently destroyed the existing getter/setter instead of
// merging into it (ValidateAndApplyPropertyDescriptor, ES2025 10.1.6.3: a
// descriptor that mentions none of Value/Writable/Get/Set is "generic" and
// must not change the property's kind at all).
const checks: boolean[] = [];

// --- Minimal repro from the issue: a class getter flipped enumerable via a
// shared descriptor object (the undici `kEnumerableProperty` idiom). ---
class Foo {
  #signal: string;
  constructor() {
    this.#signal = "hello";
  }
  get signal() {
    return this.#signal;
  }
}
const kEnumerableProperty = { enumerable: true };
const FooProto: any = (Foo as any).prototype;
Object.defineProperties(FooProto, {
  signal: kEnumerableProperty,
});
const f = new Foo();
checks.push(f.signal === "hello");
checks.push(
  Object.getOwnPropertyDescriptor(FooProto, "signal")!.enumerable === true
);
checks.push(
  typeof Object.getOwnPropertyDescriptor(FooProto, "signal")!.get ===
    "function"
);

// --- Object.defineProperty (singular), shared descriptor object reused
// across keys - same idiom, different entry point. ---
const o: any = {};
Object.defineProperty(o, "x", {
  get: function () {
    return 42;
  },
  configurable: true,
});
Object.defineProperty(o, "x", kEnumerableProperty);
checks.push(o.x === 42);
checks.push(Object.getOwnPropertyDescriptor(o, "x").enumerable === true);

// --- Both getter and setter must survive a generic descriptor. ---
let backing = 1;
Object.defineProperty(o, "y", {
  get: function () {
    return backing;
  },
  set: function (v: number) {
    backing = v;
  },
  configurable: true,
});
Object.defineProperty(o, "y", { configurable: false });
o.y = 99;
checks.push(backing === 99 && o.y === 99);
checks.push(Object.getOwnPropertyDescriptor(o, "y").configurable === false);

// --- A generic descriptor on a non-configurable accessor is still a
// no-op success when it doesn't actually change anything ... ---
let noThrow = true;
try {
  Object.defineProperty(o, "y", {});
} catch (e) {
  noThrow = false;
}
checks.push(noThrow);

// --- ... but still rejects an actual attribute change once locked down. ---
let threw = false;
try {
  Object.defineProperty(o, "y", { configurable: true });
} catch (e) {
  threw = e instanceof TypeError;
}
checks.push(threw);

// --- A descriptor that DOES specify value/writable must still convert an
// accessor into a real data property (the generic-descriptor carve-out must
// not swallow legitimate kind conversions). ---
const p: any = {};
Object.defineProperty(p, "z", {
  get: function () {
    return 1;
  },
  configurable: true,
});
Object.defineProperty(p, "z", { value: 5, writable: true, configurable: true });
checks.push(p.z === 5);
checks.push(
  typeof Object.getOwnPropertyDescriptor(p, "z").get === "undefined"
);

// --- A generic descriptor that merely restates the current attributes of a
// NON-configurable accessor must still succeed as a no-op (this exercises
// accessorRedefineAllowed with hasGetter=hasSetter=false, the exact path the
// fix routes into for a locked-down accessor). ---
const q: any = {};
Object.defineProperty(q, "w", {
  get: function () {
    return 7;
  },
  enumerable: false,
  configurable: false,
});
Object.defineProperty(q, "w", { enumerable: false });
checks.push(q.w === 7);
checks.push(Object.getOwnPropertyDescriptor(q, "w").enumerable === false);

// --- Same generic-descriptor merge, through a symbol key. ---
const sym = Symbol("k");
const r: any = {};
Object.defineProperty(r, sym, {
  get: function () {
    return 55;
  },
  configurable: true,
});
Object.defineProperty(r, sym, { enumerable: true });
checks.push(r[sym] === 55);
checks.push(Object.getOwnPropertyDescriptor(r, sym).enumerable === true);
checks.push(typeof Object.getOwnPropertyDescriptor(r, sym).get === "function");

checks.every((c) => c === true);

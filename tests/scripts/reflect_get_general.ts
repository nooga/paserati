// expect: true
// Reflect.get (pkg/builtins/reflect_init.go) used to only handle
// target.Type() == TypeObject/TypeDictObject/TypeArray - every other kind
// (Function, Closure, Map, Set, RegExp, BoundFunction, NativeFunction,
// NativeFunctionWithProps, Promise, TypedArray, Proxy, ...) silently fell
// through and returned undefined instead of actually reading the
// property. It also unconditionally stringified the key
// (`args[1].ToString()`), so Reflect.get(anything, someSymbol) never
// worked correctly except via incidental string coercion:
//
//   function fn() {}
//   Reflect.get(fn, "name");        // before: undefined - Node: "fn"
//   Reflect.get(fn, "call");        // before: undefined - Node: [Function: call]
//   Reflect.get(new Map(), "size"); // before: undefined - Node: 0
//   Reflect.get({x: 1}, "x");       // before: 1 (correct) - Node: 1
//
// Fixed by rewriting Reflect.get to delegate to two new general-purpose
// VM helpers (pkg/vm/vm_init.go) instead of re-implementing per-kind
// property lookup a third time (opGetProp/opGetPropSymbol already do this
// for bytecode dispatch, and GetProperty already did a narrower version
// for a handful of other native-function call sites):
//   - GetPropertyWithReceiver (string keys) - an existing GetProperty
//     function, extended with a `receiver` parameter (threaded through
//     every accessor-getter call and prototype-chain recursion) and two
//     missing kinds (TypeNativeFunction, TypeDictObject).
//   - ReflectGetSymbolPropertyWithReceiver (symbol keys) - new, mirroring
//     GetPropertyWithReceiver's same per-kind case list with the
//     string-keyed PlainObject methods swapped for their *ByKey
//     counterparts.
// receiver is Reflect.get's optional third argument - the spec's `this`
// for any accessor's getter along the way, independent of which object
// on the prototype chain the getter was actually found on. Reflect.get's
// declared type signature only allowed 2 arguments before this fix
// (Reflect.set/Reflect.construct have the identical gap for their own
// optional trailing parameters - not fixed here, flagged as a follow-up).
//
// Found and fixed alongside the above, in the same commit (same file,
// same switch statements, the same missing-side-table-check bug already
// fixed for other kinds in earlier PRs in this stack):
//   - TypeSet/TypeMap/TypePromise's own custom properties (a plain
//     `m.custom = 1` or Object.defineProperty) were invisible to
//     GetProperty/Reflect.get entirely - only the prototype chain (for
//     `size`, etc.) was ever checked.
//   - A value's [[Prototype]] chain passing through a callable
//     (Object.create(someFunction), or a class extending a native
//     constructor) stopped the walk early and answered "not found" even
//     when the callable had the property in its own Properties side
//     table - because the walk's continuation check used IsObject(),
//     which is FALSE for every callable ValueType (they sort before
//     TypeObject in the enum, pkg/vm/value.go) - this bug appeared (and
//     was caught and fixed) in an earlier draft of this exact fix, so
//     it's pinned explicitly below (check 18) rather than only in a code
//     comment.
//
// Each assertion below uses its own object/function/collection instance -
// no shared-mutable-intrinsic hazard here, unlike the native-function
// tests earlier in this stack (Array.prototype.* is a real shared
// intrinsic; a fresh literal or `new` expression is not).

const checks: boolean[] = [];

// --- 1. Plain function: name/length/call (string keys). ---
{
  function fn(a: number, b: number) {}
  checks.push(Reflect.get(fn, "name") === "fn");
  checks.push(Reflect.get(fn, "length") === 2);
  checks.push(typeof Reflect.get(fn, "call") === "function");
}

// --- 2. Map: size (an accessor on Map.prototype), and a custom own
// property (the side-table gap found while fixing this). ---
{
  const m: any = new Map([[1, 2]]);
  checks.push(Reflect.get(m, "size") === 1);
  m.custom = 42;
  checks.push(Reflect.get(m, "custom") === 42);
}

// --- 3. Set: size, and a custom own property. ---
{
  const s: any = new Set([1, 2, 3]);
  checks.push(Reflect.get(s, "size") === 3);
  s.custom = "hi";
  checks.push(Reflect.get(s, "custom") === "hi");
}

// --- 4. RegExp: source (own field) and lastIndex (own, writable). ---
{
  const re: any = /abc/gi;
  checks.push(Reflect.get(re, "source") === "abc");
  re.lastIndex = 3;
  checks.push(Reflect.get(re, "lastIndex") === 3);
}

// --- 5. BoundFunction: name/length (synthesized), and a custom own
// property. ---
{
  function orig(a: number, b: number, c: number) {}
  const bound: any = orig.bind(null, 1);
  checks.push(Reflect.get(bound, "name") === "bound orig");
  checks.push(Reflect.get(bound, "length") === 2);
  bound.custom = "b";
  checks.push(Reflect.get(bound, "custom") === "b");
}

// --- 6. A plain native function (Array.prototype.push-style): name/
// length, and a custom own property via Object.defineProperty. ---
{
  const nf: any = Array.prototype.slice;
  checks.push(Reflect.get(nf, "name") === "slice");
  checks.push(typeof Reflect.get(nf, "length") === "number");
  Object.defineProperty(nf, "custom6", { value: 7, configurable: true });
  checks.push(Reflect.get(nf, "custom6") === 7);
}

// --- 7. A class: name, an inherited static property, an instance
// method found through the prototype chain. ---
{
  class Base7 {
    static staticProp = 10;
    method() { return 1; }
  }
  class Derived7 extends Base7 {}
  const Derived7Any: any = Derived7;
  checks.push(Reflect.get(Derived7Any, "name") === "Derived7");
  checks.push(Reflect.get(Derived7Any, "staticProp") === 10);
  checks.push(typeof Reflect.get(Derived7Any.prototype, "method") === "function");
}

// --- 8. An accessor property invoked with the correct receiver
// argument (per spec, the getter's `this`), and the default-to-target
// behavior when receiver is omitted. ---
{
  const proto8 = {
    get val(): number { return (this as any).backing; },
    set val(v: number) { (this as any).backing = v; },
  };
  const receiver8: any = { backing: 5 };
  checks.push(Reflect.get(proto8, "val", receiver8) === 5);
  const obj8: any = Object.create(proto8);
  obj8.backing = 9;
  checks.push(Reflect.get(obj8, "val") === 9);
}

// --- 9. String and Symbol keys on the same plain object, including an
// own accessor keyed by a Symbol. ---
{
  const sym9 = Symbol("k9");
  let backing9 = 0;
  const obj9: any = {
    get [sym9]() { return backing9; },
    set [sym9](v: number) { backing9 = v; },
    plain: "p",
  };
  obj9[sym9] = 55;
  checks.push(Reflect.get(obj9, sym9) === 55);
  checks.push(Reflect.get(obj9, "plain") === "p");
}

// --- 10. Symbol key as an own property on a native function. ---
{
  const nf2: any = Array.prototype.unshift;
  const sym10 = Symbol("k10");
  Object.defineProperty(nf2, sym10, { value: 100, configurable: true });
  checks.push(Reflect.get(nf2, sym10) === 100);
}

// --- 11. Symbol key inherited via the prototype chain
// (Symbol.hasInstance on Function.prototype, for a plain function). ---
{
  function plainFn11() {}
  checks.push(typeof Reflect.get(plainFn11, Symbol.hasInstance) === "function");
}

// --- 12. Symbol key as a custom own property on a Map. ---
{
  const m2: any = new Map();
  const sym12 = Symbol("k12");
  Object.defineProperty(m2, sym12, { value: "sym-on-map", configurable: true });
  checks.push(Reflect.get(m2, sym12) === "sym-on-map");
}

// --- 13. TypedArray: numeric index, "length", and an inherited method. ---
{
  const ta: any = new Int32Array([10, 20, 30]);
  checks.push(Reflect.get(ta, "0") === 10);
  checks.push(Reflect.get(ta, "length") === 3);
  checks.push(typeof Reflect.get(ta, "map") === "function");
}

// --- 14. Proxy: the get trap is invoked as handler.get(target, key,
// receiver) for both a string and a Symbol key, with the actual
// receiver argument passed through (not the proxy itself when a
// distinct receiver is given). ---
{
  let seenReceiver14: any = null;
  const target14: any = {};
  const handler14 = {
    get(t: any, k: any, r: any) {
      seenReceiver14 = r;
      return "trapped:" + String(k);
    },
  };
  const p14: any = new Proxy(target14, handler14);
  const rcv14 = {};
  checks.push(Reflect.get(p14, "x", rcv14) === "trapped:x");
  checks.push(seenReceiver14 === rcv14);
  const sym14 = Symbol("k14");
  checks.push(Reflect.get(p14, sym14) === "trapped:Symbol(k14)");
}

// --- 15. Proxy with no trap: falls through to the target. ---
{
  const target15: any = { y: 123 };
  const p15: any = new Proxy(target15, {});
  checks.push(Reflect.get(p15, "y") === 123);
}

// --- 16. Absent property on a plain object answers undefined, no throw. ---
{
  checks.push(Reflect.get({}, "nope") === undefined);
}

// --- 17. Enum (a TypeDictObject at runtime) as a Reflect.get target -
// this case didn't exist at all before this fix, so this exercises it
// directly rather than only via the shared machinery's other callers. ---
{
  enum Color17 { Red, Green, Blue }
  checks.push(Reflect.get(Color17 as any, "Red") === 0);
  checks.push(Reflect.get(Color17 as any, "Blue") === 2);
}

// --- 18. A value's own [[Prototype]] chain passes through a callable
// (Object.create(fn)) - both key kinds must still find an own property
// on that callable's own table, not stop early. This is the bug an
// earlier draft of this fix had (the walk's continuation check used
// IsObject(), which is false for every callable kind - see this file's
// header comment) - pinned explicitly here, not just documented in code. ---
{
  function base18() {}
  (base18 as any).regular = "r";
  const sym18 = Symbol("s18");
  (base18 as any)[sym18] = "found18";
  const inst18: any = Object.create(base18);
  checks.push(Reflect.get(inst18, "regular") === "r");
  checks.push(Reflect.get(inst18, sym18) === "found18");
}

checks.every((c) => c === true);

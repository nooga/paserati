// Object.freeze / Object.seal / defineProperty must not re-attribute properties
// on unrelated objects that merely share the same hidden shape (issue #122).
// expect: pass

const results: string[] = [];

function check(name: string, actual: any, expected: any) {
  if (actual !== expected) {
    results.push(name + ": got " + String(actual) + ", want " + String(expected));
  }
}

// The original repro: a frozen value flowing into obj.x used to leave the
// (obj, "x") slot itself non-writable, so the second write was a silent no-op.
const source = Object.freeze({ x: Object.freeze([1]) });
const obj: any = {};
obj.x = source.x;
obj.x = [9, 9];
obj.x.push(3);
check("repro", JSON.stringify(obj.x), "[9,9,3]");

// Freezing one object must not make a sibling's property read-only, whether the
// sibling was created before or after the freeze.
const before: any = { w: 1 };
const frozen: any = { w: 1 };
Object.freeze(frozen);
const after: any = { w: 1 };
before.w = 42;
after.w = 43;
check("before.w", before.w, 42);
check("after.w", after.w, 43);
check("frozen.w", frozen.w, 1);
check("isFrozen(before)", Object.isFrozen(before), false);
check("isFrozen(frozen)", Object.isFrozen(frozen), true);

// ... and the descriptor of an untouched sibling stays fully default.
const desc: any = Object.getOwnPropertyDescriptor(after, "w");
check("desc.writable", desc.writable, true);
check("desc.enumerable", desc.enumerable, true);
check("desc.configurable", desc.configurable, true);

// defineProperty making a property non-enumerable must not hide it elsewhere.
const hidden: any = { k: 1 };
const visible: any = { k: 1 };
Object.defineProperty(hidden, "k", { enumerable: false });
check("keys(visible)", JSON.stringify(Object.keys(visible)), '["k"]');
check("keys(hidden)", JSON.stringify(Object.keys(hidden)), "[]");

// Object.seal likewise.
const sealed: any = { s: 1 };
const open: any = { s: 1 };
Object.seal(sealed);
check("isSealed(open)", Object.isSealed(open), false);
check("delete open.s", delete open.s, true);
check("delete sealed.s", Object.isSealed(sealed), true);

// Sealing preserves writable; freezing a sealed object still takes it away.
const seal2: any = { p: 1 };
Object.seal(seal2);
seal2.p = 5;
check("sealed writable", seal2.p, 5);
check("isFrozen(sealed)", Object.isFrozen(seal2), false);
Object.freeze(seal2);
check("isFrozen after freeze", Object.isFrozen(seal2), true);
// Test scripts run in strict mode, so the rejected write throws.
let threw = false;
try {
  seal2.p = 7;
} catch (e) {
  threw = true;
}
check("frozen write throws", threw, true);
check("frozen not writable", seal2.p, 5);

// Freezing twice must be idempotent (the second freeze hits the memoized
// transition and must not disturb the value).
const twice: any = { t: 1 };
Object.freeze(twice);
Object.freeze(twice);
check("double freeze", Object.isFrozen(twice), true);
check("double freeze value", twice.t, 1);

// Object.freeze / Object.seal reach a function's or a RegExp's own properties,
// which live in a side table rather than on the value itself.
const fn: any = function () {};
fn.foo = 10;
Object.seal(fn);
const fnDesc: any = Object.getOwnPropertyDescriptor(fn, "foo");
check("sealed fn configurable", fnDesc.configurable, false);
check("sealed fn writable", fnDesc.writable, true);
check("isSealed(fn)", Object.isSealed(fn), true);
const re: any = /x/;
re.bar = 10;
Object.freeze(re);
const reDesc: any = Object.getOwnPropertyDescriptor(re, "bar");
check("frozen re configurable", reDesc.configurable, false);
check("frozen re writable", reDesc.writable, false);

// A built-in constructor's "prototype" is non-writable, non-enumerable and
// non-configurable - each built-in now says so itself.
const ctorDesc: any = Object.getOwnPropertyDescriptor(Array, "prototype");
check("Array.prototype writable", ctorDesc.writable, false);
check("Array.prototype enumerable", ctorDesc.enumerable, false);
check("Array.prototype configurable", ctorDesc.configurable, false);
// ... while an ordinary function's stays writable.
const userDesc: any = Object.getOwnPropertyDescriptor(function () {}, "prototype");
check("fn.prototype writable", userDesc.writable, true);

// Accessor transitions are cached per shape; the cache key must include the
// attributes, or the second define inherits the first one's.
const sym = Symbol("k");
const accEnum: any = {};
Object.defineProperty(accEnum, sym, { get: () => 1, enumerable: true, configurable: true });
const accPlain: any = {};
Object.defineProperty(accPlain, sym, { get: () => 2, enumerable: false, configurable: false });
const d1: any = Object.getOwnPropertyDescriptor(accEnum, sym);
const d2: any = Object.getOwnPropertyDescriptor(accPlain, sym);
check("acc1.enumerable", d1.enumerable, true);
check("acc1.configurable", d1.configurable, true);
check("acc2.enumerable", d2.enumerable, false);
check("acc2.configurable", d2.configurable, false);
check("acc1 value", accEnum[sym], 1);
check("acc2 value", accPlain[sym], 2);

results.length === 0 ? "pass" : results.join("; ");

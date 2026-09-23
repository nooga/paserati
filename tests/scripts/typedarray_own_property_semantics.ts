// expect: [["0","1","x"],["0","1","x"],1,4,{"value":0,"writable":true,"enumerable":true,"configurable":true},true,false] | 44,TypeError,TypeError,TypeError,false | 0,1,x {"0":44,"1":0,"x":1} true false | true,true,true true,false,false,false,true,false | TypeError,true,false,true,true,true | true,true,true,true,true,true
// skip-typecheck
// paserati#528: TypedArray own-property semantics - integer indices are own
// properties (listed first), named expandos live on the side table, a
// canonical numeric key never consults the prototype (10.4.5), freeze throws
// while elements exist. Plus Object.getPrototypeOf, which answered null for
// WeakSet/WeakRef and for number/boolean/symbol/bigint primitives.
const out = [];
const t = (f) => { try { return String(f()); } catch (e) { return e.constructor.name; } };
const u = new Uint8Array(2); u.x = 1; const s = Symbol("s"); u[s] = 2;
out.push(JSON.stringify([Object.keys(u), Object.getOwnPropertyNames(u), Object.getOwnPropertySymbols(u).length, Reflect.ownKeys(u).length, Object.getOwnPropertyDescriptor(u, "0"), Object.hasOwn(u, "0"), Object.hasOwn(u, "5")]));
out.push([t(() => Object.defineProperty(u, "0", { value: 300 }) && u[0]), t(() => Object.defineProperty(u, "5", { value: 1 })), t(() => Object.defineProperty(u, "1", { get() { return 1; } })), t(() => Object.defineProperty(u, "1", { value: 1, writable: false })), Reflect.defineProperty(u, "-0", { value: 1 })].join());
const ks = []; for (const k in u) ks.push(k);
out.push(ks.join() + " " + JSON.stringify({ ...u }) + " " + Reflect.set(u, "7", 1) + " " + Reflect.has(u, "7"));
const v = new Uint8Array(2); const r0 = [0 in v, "length" in v, "BYTES_PER_ELEMENT" in v];
v.__proto__ = { 5: 1, "1.5": 1, x: 1 };
out.push(r0.join() + " " + [0 in v, 5 in v, "1.5" in v, "-0" in v, "x" in v, "length" in v].join());
const w = new Uint8Array(1), e0 = new Uint8Array(0);
out.push([t(() => Object.freeze(w) === w), t(() => Object.seal(w) === w), Object.isFrozen(w), Object.isSealed(w), t(() => Object.freeze(e0) === e0), Object.isFrozen(e0)].join());
out.push([Object.getPrototypeOf(new WeakSet()) === WeakSet.prototype, Object.getPrototypeOf(new WeakRef({})) === WeakRef.prototype, Object.getPrototypeOf(1) === Number.prototype, Object.getPrototypeOf(true) === Boolean.prototype, Object.getPrototypeOf(Symbol()) === Symbol.prototype, Object.getPrototypeOf(1n) === BigInt.prototype].join());
out.join(" | ");

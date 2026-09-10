// expect: 0
// Symbol.species accessors on the built-in constructors (issue #381).
// Each species-aware constructor carries a `get [Symbol.species]` whose getter
// returns `this`, so the property is both directly readable and correctly
// inherited by subclasses.

let fails = 0;
function chk(name: string, cond: boolean): void {
  if (!cond) {
    fails++;
    console.log("FAIL", name);
  }
}

const ctors: any[] = [Array, ArrayBuffer, SharedArrayBuffer, Map, Set, Promise, RegExp];
for (const C of ctors) {
  chk(C.name + " returns itself", C[Symbol.species] === C);
  const d: any = Object.getOwnPropertyDescriptor(C, Symbol.species);
  chk(C.name + " is an accessor", typeof d.get === "function");
  chk(C.name + " has no setter", d.set === undefined);
  chk(C.name + " non-enumerable", d.enumerable === false);
  chk(C.name + " configurable", d.configurable === true);
  chk(C.name + " getter name", d.get.name === "get [Symbol.species]");
  chk(C.name + " getter length", d.get.length === 0);
}

// The TypedArray constructors inherit the accessor from %TypedArray%: it must
// answer the subclass, and must NOT appear as an own property of each of them.
chk("Uint8Array", Uint8Array[Symbol.species] === Uint8Array);
chk("Int32Array", Int32Array[Symbol.species] === Int32Array);
chk("Float64Array", Float64Array[Symbol.species] === Float64Array);
chk("no own species on Uint8Array", Object.getOwnPropertyDescriptor(Uint8Array, Symbol.species) === undefined);

// DataView is not species-aware per spec.
chk("DataView has none", (DataView as any)[Symbol.species] === undefined);

// User subclasses inherit the getter and answer themselves.
class MyBytes extends Uint8Array {}
class MyList extends Array {}
class MyPromise extends Promise {}
chk("subclass of Uint8Array", (MyBytes as any)[Symbol.species] === MyBytes);
chk("subclass of Array", (MyList as any)[Symbol.species] === MyList);
chk("subclass of Promise", (MyPromise as any)[Symbol.species] === MyPromise);

// ...until it is overridden.
class Overridden extends Uint8Array {
  static get [Symbol.species]() {
    return Uint8Array;
  }
}
chk("override wins", (Overridden as any)[Symbol.species] === Uint8Array);

// The undici pattern from the issue: read the species off a TypedArray
// constructor and use it to build a view.
const FastBuffer: any = Uint8Array[Symbol.species];
const view = new FastBuffer(new ArrayBuffer(8), 0, 4);
chk("species-constructed view", view.length === 4 && view instanceof Uint8Array);

// Species-aware methods must build their result through the species
// constructor, which means the native read path has to resolve a subclass
// instance's own [[Prototype]] rather than the intrinsic one.
class Bytes extends Uint8Array {}
const b: any = new Bytes([1, 2, 3, 4]);
chk("typed array filter", b.filter((v: number) => v > 1) instanceof Bytes);
chk("typed array map", b.map((v: number) => v) instanceof Bytes);
chk("typed array slice", b.slice(0, 2) instanceof Bytes);
chk("typed array subarray", b.subarray(0, 2) instanceof Bytes);

class PlainBytes extends Uint8Array {
  static get [Symbol.species]() {
    return Uint8Array;
  }
}
const pb: any = new PlainBytes([1, 2, 3, 4]);
chk("species override wins over subclass", pb.slice(0, 2).constructor === Uint8Array);

// Promise.prototype.then/catch/finally do SpeciesConstructor(promise, %Promise%).
class Chained extends Promise {}
const cp: any = new Chained((resolve: any) => resolve(1));
chk("promise then", cp.then((x: number) => x) instanceof Chained);
chk("promise catch", cp.catch(() => {}) instanceof Chained);
chk("promise finally", cp.finally(() => {}) instanceof Chained);
chk("plain promise then", Promise.resolve(1).then((x) => x) instanceof Promise);

class PlainChained extends Promise {
  static get [Symbol.species]() {
    return Promise;
  }
}
const pc: any = new PlainChained((resolve: any) => resolve(1));
chk("promise species override", pc.then((x: number) => x).constructor === Promise);

// A user-defined (bytecode) species getter reached from a native method must
// see the right `this`: %TypedArray%.prototype.filter resolves species through
// the VM's native symbol-get path, not the bytecode one.
let seenThis: any = "getter not called";
class Reporting extends Uint8Array {
  static get [Symbol.species]() {
    seenThis = this;
    return Uint8Array;
  }
}
const plain: any = new Uint8Array([1, 2, 3, 4]);
plain.constructor = Reporting;
chk("native path reaches species", plain.filter((v: number) => v > 1).length === 3);
chk("native path passes the right this", seenThis === Reporting);

// Honoring a per-instance [[Prototype]] on the native read path must not assume
// that prototype is a plain object: Reflect.construct with a newTarget whose
// .prototype is a Proxy (or a dictionary-mode object) puts an exotic value in
// that slot, and reading through it used to crash the VM.
function ProxyProto(): void {}
(ProxyProto as any).prototype = new Proxy({}, {});
const exotic: any = Reflect.construct(Uint8Array, [4], ProxyProto as any);
chk("exotic prototype survives a native read", (Uint8Array.prototype as any).filter.call(exotic, () => true).length === 4);

// Symbol-keyed accessors on ordinary functions must work the same way, own and
// inherited - that read path is what made the built-in accessors observable.
const marker = Symbol("marker");
class Base {}
Object.defineProperty(Base, marker, {
  get() {
    return this;
  },
});
class Derived extends Base {}
chk("own symbol accessor on a constructor", (Base as any)[marker] === Base);
chk("inherited symbol accessor on a subclass", (Derived as any)[marker] === Derived);

fails;

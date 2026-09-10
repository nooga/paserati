// expect: 0
// Promise.resolve per spec (PromiseResolve(C, x)): honors `this`, and feeds the
// value through a promise capability's resolve function so a thenable is
// assimilated instead of becoming the fulfillment value. Plus `instanceof
// Function` for the callable kinds that never reached the prototype walk.

let fails = 0;
function chk(name: string, cond: boolean): void {
  if (!cond) {
    fails++;
    console.log("FAIL", name);
  }
}

// Functions are objects: every callable kind must walk its prototype chain.
chk("native method", (Map.prototype.get as any) instanceof Function);
chk("builtin constructor", (Map as any) instanceof Function);
chk("bound function", (function () {}).bind(null) instanceof Function);
chk("user function", function () {} instanceof Function);
chk("arrow function", (() => {}) instanceof Function);
// Uint8Array reaches Function.prototype only through %TypedArray%.
chk("typed array constructor", (Uint8Array as any) instanceof Function);
chk("function is an Object too", (function () {} as any) instanceof Object);
// ...and primitives still are not.
chk("number is not an Object", !((5 as any) instanceof Object));
chk("string is not an Object", !(("s" as any) instanceof Object));

// Promise.resolve uses `this` as the constructor.
class SubPromise extends Promise {}
chk("subclass resolve", (SubPromise as any).resolve(1) instanceof SubPromise);
chk("subclass statics reachable", typeof (SubPromise as any).resolve === "function");

// A promise already constructed by C passes through identically; one built by a
// different constructor does not.
const p: any = Promise.resolve(5);
chk("identity for same constructor", Promise.resolve(p) === p);
chk("no identity across constructors", (SubPromise as any).resolve(p) !== p);

// Thenable assimilation is inherently asynchronous - the `then` CALL is a
// microtask - and this harness compares the last statement's value before the
// microtask queue drains, so the outcome of an assimilated thenable cannot be
// asserted here. test262 covers it (built-ins/Promise/resolve/*,
// built-ins/Promise/prototype/finally/*). What IS synchronously observable is
// that resolving reads `then` on this tick, which is the step that decides
// between assimilating and fulfilling with the object.
let thenWasRead = false;
const probe: any = {};
Object.defineProperty(probe, "then", {
  get() {
    thenWasRead = true;
    return undefined;
  },
});
Promise.resolve(probe);
chk("resolving reads `then` synchronously", thenWasRead);

fails;

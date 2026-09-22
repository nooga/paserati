// expect: undefined  true zip.resume
// skip-typecheck
// paserati#531, real-world shapes: @vitest/expect's module-level
// `if (!hasOwn(globalThis, X)) { const globalState = new WeakMap(); ... get: () => ({state: globalState.get(...)}) }`
// and tar v7's Pack `if (t.gzip) { let e = this.zip; this.on("resume", () => e.resume()); } this[Ee] = !1`.
const MATCHERS_OBJECT = Symbol.for("matchers-object-test");
const JEST_MATCHERS_OBJECT = Symbol.for("jest-matchers-object-test");
const GLOBAL_EXPECT = Symbol.for("global-expect-test");
if (!Object.prototype.hasOwnProperty.call(globalThis, MATCHERS_OBJECT)) {
  const globalState = new WeakMap();
  const matchers = Object.create(null);
  const customEqualityTesters = [];
  Object.defineProperty(globalThis, MATCHERS_OBJECT, { get: () => globalState.get(globalThis[GLOBAL_EXPECT]) });
  Object.defineProperty(globalThis, JEST_MATCHERS_OBJECT, {
    configurable: true,
    get: () => ({ state: globalState.get(globalThis[GLOBAL_EXPECT]), matchers, customEqualityTesters })
  });
}
const jm = globalThis[JEST_MATCHERS_OBJECT];
const out = [typeof jm.state, Object.getPrototypeOf(jm.matchers), Array.isArray(jm.customEqualityTesters)];

// tar Pack shape (minified)
const { EventEmitter } = { EventEmitter: class { constructor() { this.h = {}; } on(n, f) { (this.h[n] ||= []).push(f); } emit(n) { (this.h[n] || []).forEach(f => f()); } } };
const Ee = Symbol("Ee"), me = Symbol("me");
class Pack extends EventEmitter {
  constructor(t) {
    super();
    this.zip = { resume() { out.push("zip.resume"); } };
    if (t.gzip) { let e = this.zip; this.on("resume", () => e.resume()); }
    this[Ee] = !1, this[me] = !1;
  }
}
new Pack({ gzip: true }).emit("resume");
out.join(" ");

// Regression: OpSetIndex skips the Array.prototype index-setter walk via a latch
// (arrayIndexAccessorSeen) that must flip when an integer-indexed accessor is
// defined on the prototype chain. Verifies inherited index setters still fire.
// expect: true
let called = 0;
let lastVal: any = null;
Object.defineProperty(Array.prototype, "5", {
  set: function (v: any) { called++; lastVal = v; },
  configurable: true,
});
const a: any[] = [];
a[5] = 999; // must invoke the inherited setter, not store directly
// setter-only accessor: the write is swallowed, so reading a[5] yields undefined
const setterFired = called === 1 && lastVal === 999 && a[5] === undefined;
a[6] = 111; // no setter for index 6 -> normal store
const normalStore = a[6] === 111;
// clean up so the global prototype mutation doesn't leak to other scripts
delete (Array.prototype as any)["5"];
setterFired && normalStore;

// expect: true
// skip-typecheck
// Bare (unqualified) references to Object.prototype methods must resolve via
// the global object's prototype chain, same as `globalThis.hasOwnProperty`
// does. See #246. Uses skip-typecheck (not no-typecheck) because that's the
// code path real JS (paserati --no-typecheck / noderati) actually takes -
// the checker's global environment happens to already know these names, so
// no-typecheck alone would silently exercise a different path than the bug.

let results: boolean[] = [];

results.push(typeof hasOwnProperty === "function");
results.push(typeof toString === "function");
results.push(typeof valueOf === "function");
results.push(typeof isPrototypeOf === "function");
results.push(typeof propertyIsEnumerable === "function");
results.push(typeof toLocaleString === "function");

results.push(hasOwnProperty.call({ a: 1 }, "a") === true);
results.push(hasOwnProperty.call({ a: 1 }, "b") === false);

results.every((r) => r === true);

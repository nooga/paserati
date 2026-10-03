// expect: 1|ReferenceError|false|completed-before-throw|ReferenceError
// no-typecheck
let count = 0;
let caught = "none";
Object.defineProperty(globalThis, "sx", {
  configurable: true,
  get: function () { delete (globalThis as any).sx; return 2; },
});
(function () {
  "use strict";
  try { count++; sx ^= 3; count++; } catch (e: any) { caught = e.name; }
})();
// A plain assignment to an unresolvable name still throws, but only after its RHS ran.
let order = "none";
let err2 = "none";
(function () {
  "use strict";
  try { sy = (order = "completed-before-throw"); } catch (e: any) { err2 = e.name; }
})();
[count, caught, "sx" in globalThis, order, err2].join("|");

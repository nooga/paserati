// expect: 10|11|12|number|undefined|undefined|false|ReferenceError:neverDefined585 is not defined
// no-typecheck
// paserati#585: strict-mode eval couldn't read a global created at runtime as
// a globalThis property. The eval paths synced global names to the VM only for
// sloppy code, so the strict chunk's slot had no name and OpGetGlobal could not
// fall back to the global object ("<index N> is not defined"). Strict eval's
// own declarations must still stay local.
globalThis.runtimeGlobal585 = 10;
const r = [];
r.push((0, eval)('"use strict"; runtimeGlobal585'));
r.push(eval('"use strict"; runtimeGlobal585 + 1'));
r.push((0, eval)('"use strict"; (() => runtimeGlobal585 + 2)()'));
r.push((0, eval)('"use strict"; typeof runtimeGlobal585'));
(0, eval)('"use strict"; var strictLocal585 = 1; function strictFn585() {}');
r.push(typeof strictLocal585, typeof strictFn585, "strictLocal585" in globalThis);
let threw = "no";
try {
  (0, eval)('"use strict"; neverDefined585');
} catch (e) {
  threw = e.constructor.name + ":" + e.message;
}
r.push(threw);
r.join("|");

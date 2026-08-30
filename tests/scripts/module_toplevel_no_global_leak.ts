// expect: hi,41,3,old,7,undefined,undefined,undefined,undefined,undefined,ReferenceError,ReferenceError,9
// skip-typecheck
// Regression test for #106: every top-level function/let/const/var - and a
// standalone named class expression used as `export default class Named {}`
// (compileClassExpression, the one case #105's own class fix missed) - must
// not install a bare-name heap global visible to every other module, exactly
// like #103/#105 already fixed for `export class Name {}` declarations.
//
// Note: the local aliases below (g, c, pi, l, A) deliberately avoid the
// helper's bare names (greet, counter, PI_ISH, legacy, Agent) so those bare
// names stay genuinely un-imported in this module, for the checks below.
import {
  greet as g,
  counter as c,
  PI_ISH as pi,
  legacy as l,
} from "./module_toplevel_no_global_leak_helper.ts";
import A from "./module_toplevel_no_global_leak_helper.ts";

function unresolvedBareRefs() {
  // References elsewhere in this module to the bare (never-imported) names
  // must not "warm up" a shared slot for the checks below (see #103/#107).
  return [typeof greet, typeof counter, typeof PI_ISH, typeof legacy, typeof Agent];
}

let bareCallThrew = false;
try {
  greet();
} catch (e) {
  bareCallThrew = e instanceof ReferenceError;
}

let bareConstructThrew = false;
try {
  new Agent();
} catch (e) {
  bareConstructThrew = e instanceof ReferenceError;
}

// A plain top-level class in a script (no imports/exports at all) is
// unaffected and still installs a real global, as before this fix.
class Local {
  n() {
    return 9;
  }
}

[
  g(),
  c,
  pi,
  l,
  new A().getN(),
  ...unresolvedBareRefs(), // typeof greet, counter, PI_ISH, legacy, Agent: all "undefined"
  bareCallThrew ? "ReferenceError" : "called",
  bareConstructThrew ? "ReferenceError" : "constructed",
  typeof Local === "function" ? new Local().n() : "not global",
].join(",");

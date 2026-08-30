// expect: function,7,undefined,undefined,ReferenceError,9
// skip-typecheck
// Regression test for #103: a top-level `export class` must not install a
// heap global visible to every other module by its bare name. Before the
// fix, `import { Agent as A }` (note: NOT importing the bare name `Agent`)
// left `Agent` constructible and `typeof Agent === "function"` in the
// importer, purely because the defining module's class declaration and any
// unrelated module's implicit-global fallback shared the same global heap
// slot by name.
import { Agent as A } from "./module_class_no_global_leak_helper.ts";

function unresolvedBareRef() {
  // A reference elsewhere in this module to the bare (never-imported) name
  // must not "warm up" a shared slot for the earlier typeof/new checks below.
  return typeof Agent;
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
  typeof A, // the actual import alias: still constructs
  new A().getN(),
  typeof Agent, // bare, un-imported name: must stay "undefined"
  unresolvedBareRef(),
  bareConstructThrew ? "ReferenceError" : "constructed",
  typeof Local === "function" ? new Local().n() : "not global",
].join(",");

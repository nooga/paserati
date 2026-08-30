// expect: log1,log2,undefined
// skip-typecheck
// Regression test for #103: two unrelated modules each declaring `export
// class Logger` must not alias the same global heap slot. Before the fix,
// a module-scope class's heap slot was keyed by its plain name, so both
// siblings' OpSetGlobal wrote the same slot and an unrelated, never-imported
// bare `Logger` reference elsewhere in this module (forced into the heap
// allocator here via an unreachable reference, so the check doesn't depend
// on incidental compile/sync ordering) would leak whichever sibling ran
// last as a constructible global.
import { Logger as L1 } from "./module_class_sibling_names_helper1.ts";
import { Logger as L2 } from "./module_class_sibling_names_helper2.ts";

function unreachableBareRef() {
  return Logger;
}

[new L1().tag(), new L2().tag(), typeof Logger].join(",");

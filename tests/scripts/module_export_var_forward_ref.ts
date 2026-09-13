// paserati#451 - a top-level function that forward-references a var
// declared later in the same file threw "is not defined" when that file was
// loaded as an imported module (it worked fine as the entry script), because
// collectVarDeclarations didn't unwrap `export var` the way
// collectLetConstDeclarations already unwraps `export let`/`export const`
// (paserati#117) - so the exported var's pre-registration under its
// namespaced module-global key never happened, and the forward reference
// resolved to the wrong (bare, never-written) heap slot.
// expect: number
// no-typecheck
import { makeThing } from "./module_export_var_forward_ref_helper.ts";

makeThing();

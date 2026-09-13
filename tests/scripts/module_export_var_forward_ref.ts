// expect: number
// paserati#451: a top-level function that forward-references an `export var`
// declared later in the same file threw "ReferenceError: X is not defined"
// once that file was loaded as an imported module (it worked fine as the
// entry script) - a compiler bug, since fixed in collectVarDeclarations.
//
// The same shape also failed at the type-checker level (independent of the
// compiler bug, and reproducing even for a single, non-imported file): the
// checker's Pass 2 pre-pass, which hoists top-level Let/Const/Var names
// before Pass 3 checks function bodies, never looked inside
// ExportNamedDeclaration - so `export var`/`export let`/`export const` never
// got hoisted at all, and `Kind` below was "Cannot find name" even though
// the identical non-exported declaration works.
//
// This test exercises both fixes together, with type checking on (the
// default): the shape from the linked issue, loaded as an imported module.
import { makeThing } from "./module_export_var_forward_ref_helper.ts";

makeThing();

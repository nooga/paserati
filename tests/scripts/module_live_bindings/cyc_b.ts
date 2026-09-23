// skip-typecheck
import * as A from "./cyc_a.ts";
export let b = "b-init";
export function readA() { return A.a; }
b = "b-updated";

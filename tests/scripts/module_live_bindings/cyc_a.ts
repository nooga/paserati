// skip-typecheck
import * as B from "./cyc_b.ts";
export let a = "a-init";
export function readB() { return B.b; }
export const seenFromA = typeof B;

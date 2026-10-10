// `export declare ...` exports types (they were not exported at all) and no
// runtime values: the module namespace has only y.
import { y } from "./export_declare_helper.ts";
import * as all from "./export_declare_helper.ts";
type K = all.util.Keep<string>;
const k: K = "k";
`${y + 1} ${Object.keys(all).join(",")} ${k}`;

// expect: 42 y k

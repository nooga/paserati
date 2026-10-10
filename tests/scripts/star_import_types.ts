// import * as z: z.T in a type position names an exported type, including
// keyword names (z.infer, as in zod) and generics, and types exported
// through an `export type { A as B }` list.
import * as z from "./star_import_types_helper.ts";
type S = { _o: { n: number } };
const a: z.infer<S> = { n: 1 };
const b: z.default = 2;
const c: z.Box = { v: "x" };
const d: z.Direct = true;
`${a.n + b}${c.v}${d}${z.marker}`;

// expect: 3xtrue1

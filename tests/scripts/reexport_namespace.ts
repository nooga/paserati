// A namespace re-exported with export { z } keeps both sides: z.make() as a
// value and z.Pair<T> as a type.
import { z } from "./reexport_namespace_helper.ts";
const p: z.Pair<number> = z.make(2);
p[0] + p[1];

// expect: 4

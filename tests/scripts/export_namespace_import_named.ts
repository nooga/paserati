// expect: 3
// paserati#424: `export { ns }` where `ns` is bound via `import * as ns`
// previously failed checker-side ("exported name 'ns' not found in current
// scope") because unresolved namespace imports were never given a value
// binding, and (once that's fixed) also failed compiler-side because
// namespace imports are deliberately excluded from the "re-export via
// import" path (paserati#163) since "*" isn't a real export name to look up.
// This is the exact real-world shape from zod/prettier's compiled output:
// `import * as ns from "./somewhere"; export { ns }; export default ns;`
import * as ns from "./export_namespace_import_named_lib.ts";

export { ns };
export default ns;

ns.a + ns.b;

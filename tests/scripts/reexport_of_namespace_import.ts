// expect: 1
// paserati#424: re-exporting a *namespace* import (import * as NS; export
// { NS };) is a common real-world pattern (e.g. zod's real index.js does
// `import * as z from "./v3/external"; export { z };`). The namespace object
// is materialized into a global slot exactly like "export * as ns from
// 'module'" already did, and the re-exporting module's own named export
// resolves to that slot (superseding paserati#163's "not found" error, which
// predated this being implemented).
import { NS } from "./reexport_of_namespace_import_index.ts";

NS.Foo;

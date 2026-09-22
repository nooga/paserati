// expect: TYPE_FN,TYPE_FN
// skip-typecheck
// paserati#505: `import { type as osType } from "..."` used to fail to
// parse ('}' expected) - `type` immediately followed by `as` must be treated
// as an ordinary imported binding named "type", aliased to "osType" (not
// the type-only-import modifier), matching real Node/tsc and real published
// code like chokidar's `import { type as osType } from 'node:os'`.
import { type as osType } from "./import_type_as_alias_helper.ts";
import { type } from "./import_type_as_alias_helper.ts";
[osType(), type()].join(",");

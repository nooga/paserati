// expect: dflt,dflt,named,named
// skip-typecheck
// `from`, `type`, and `satisfies` are contextual keywords, so they are legal
// import binding names. `import from from "m"` used to take a parser path that
// returned a nil *ImportDeclaration without recording an error; the typed nil
// reached the module loader as a non-nil Statement and was dereferenced
// (test262 language/module-code/source-phase-import fixtures).
import from from "./import_binding_named_from_helper.ts";
import type from "./import_binding_named_from_helper.ts";
import { named as satisfies } from "./import_binding_named_from_helper.ts";
import * as ns from "./import_binding_named_from_helper.ts";
[from, type, satisfies, ns.named].join(",");

// expect: default,c,F,x
// skip-typecheck
// export default: a named function declaration binds its name in the module,
// and an anonymous class or function is named "default".
import C from "./module_export_default_names_helper.ts";
export default function F() { return "x"; }
[C.name, C.tag, F.name, F()].join(",");

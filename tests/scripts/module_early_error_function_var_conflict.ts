// expect_compile_error: Identifier 'f' has already been declared
// skip-typecheck
// Module early error: top-level function declarations are lexical in a
// module, so they conflict with a var of the same name.
var f;
function f() {}
import "./module_live_bindings/empty.ts";

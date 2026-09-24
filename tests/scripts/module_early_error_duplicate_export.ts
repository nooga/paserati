// expect_compile_error: Duplicate export of 'x'
// skip-typecheck
// Module early error: ExportedNames must not contain duplicates.
var x;
export { x };
export { x as x };
import "./module_live_bindings/empty.ts";

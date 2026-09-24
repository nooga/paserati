// expect_compile_error: Unexpected token 'export'
// skip-typecheck
// Module early error: export is only allowed at the module top level.
if (true) {
  export const x = 1;
}
import "./module_live_bindings/empty.ts";

// expect_compile_error: Duplicate import attribute key
// skip-typecheck
import data from "./module_live_bindings/empty.ts" with { type: "json", type: "json" };
data;

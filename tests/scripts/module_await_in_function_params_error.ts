// expect_compile_error: await is only valid in async functions
// skip-typecheck
// Top-level await does not extend into a non-async function's parameters.
import "./module_live_bindings/empty.ts";
function f(x = await 1) { return x; }

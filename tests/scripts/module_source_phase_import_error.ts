// expect_compile_error: Source phase import is not available
// skip-typecheck
// A source text module has no source-phase representation, so importing
// it in the source phase is a link-time SyntaxError.
import source empty from "./module_live_bindings/empty.ts";
empty;

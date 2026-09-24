// expect_compile_error: does not provide an export named 'default'
// skip-typecheck
// Module linking: export * never re-exports default, so importing it by
// name from a star-only module fails before anything runs.
import { default as d } from "./export_star_helper_b.ts";
d;

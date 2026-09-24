// expect: true
// skip-typecheck
// A module's top-level this is undefined, also as seen from arrows.
import "./module_live_bindings/empty.ts";
this === undefined && (() => this)() === undefined;

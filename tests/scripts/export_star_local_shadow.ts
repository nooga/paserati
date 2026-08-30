// expect: from-y
// Regression test: a module's own local export must win over a same-named
// "export * from" candidate from another module. Per spec, star re-exports
// never override a module's own bindings.
import { shared } from "./export_star_local_shadow_barrel.ts";
shared;

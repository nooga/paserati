// expect: Bar|foo|q,1,2,3
// skip-typecheck
// Regression test for #99: nested "export * from" (c re-exports b, which
// re-exports a) must flatten all the way through - the common npm barrel
// shape (a barrel re-exporting another barrel). Requires skip-typecheck to
// exercise the AST-harvesting fallback (see export_star_flat.ts).
import { foo, Bar, q } from "./export_star_helper_c.ts";
import * as ns from "./export_star_helper_c.ts";

[Object.keys(ns).sort().join("|"), foo(), new Bar().baz(), q].join(",");

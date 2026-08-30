// expect: Bar|default|foo|q,1,2,3
// Regression test for #99: "export * as ns from" must bind a single
// namespace export built from the source module's names, unlike bare
// "export *" which flattens those names into the importer's own scope.
// Unlike bare "export *", the namespace form DOES include "default" (it
// behaves like "import * as ns from ...; export { ns };").
import { ns } from "./export_star_helper_ns.ts";

[Object.keys(ns).sort().join("|"), ns.foo(), new ns.Bar().baz(), ns.q].join(
  ",",
);

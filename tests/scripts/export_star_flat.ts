// expect: Bar|foo|q,1,2,3,undefined
// skip-typecheck
// Regression test for #99: "export * from" must re-export function, class,
// and const names, and must NOT re-export "default". Needs skip-typecheck
// (not just no-typecheck): the checker's own checkExportAllDeclaration
// already populates export names when it runs at all, so only fully
// skipping the checker exercises the AST-harvesting fallback this test
// is actually guarding.
import { foo, Bar, q, default as maybeDefault } from "./export_star_helper_b.ts";
import * as ns from "./export_star_helper_b.ts";

[
  Object.keys(ns).sort().join("|"),
  foo(),
  new Bar().baz(),
  q,
  typeof maybeDefault,
].join(",");

// expect: PARAM:1,2,MODULE:3,4
// skip-typecheck
// paserati#501: a nested function used to incorrectly resolve an identifier
// to a module-level `import` binding instead of a same-named parameter of
// an enclosing function it should have closed over - only when that name
// also happened to be imported at module scope (a plain outer variable of
// the same name shadowed correctly; only imports triggered the bug).
import { resolveHelper as resolve } from "./import_shadowed_by_nested_param_helper.ts";

function test(resolve: (...args: number[]) => string) {
  function inner() {
    return resolve(1, 2);
  }
  return inner();
}

function outer() {
  function inner() {
    return resolve(3, 4);
  }
  return inner();
}

`${test((...args) => "PARAM:" + args.join(","))},${outer()}`;

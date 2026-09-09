// expect: 0,set-threw-typeerror,delete-threw-typeerror
// Regression test for #369: Module Namespace Exotic Object [[Set]]/[[Delete]]
// via *computed* bracket access (`ns[0] = v`, `delete ns[0]`) must throw
// TypeError for an existing exported binding, same as the dot-notation path
// already did. OpSetIndex's TypeObject dispatch had the module-namespace
// [[Set]] check, but OpDeleteIndex's string-key branch was missing the
// equivalent [[Delete]] check entirely, so `delete ns[key]` silently
// "succeeded" instead of throwing in module (always-strict) code.
import * as ns from "./module_namespace_delete_index_strict_helper.ts";

let results: string[] = [];

results.push(String(ns[0]));

try {
  (ns as any)[0] = 1;
  results.push("no-throw-set");
} catch (e) {
  results.push(e instanceof TypeError ? "set-threw-typeerror" : "set-threw-other");
}

try {
  delete (ns as any)[0];
  results.push("no-throw-delete");
} catch (e) {
  results.push(e instanceof TypeError ? "delete-threw-typeerror" : "delete-threw-other");
}

results.join(",");

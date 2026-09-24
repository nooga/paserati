// expect: before:,[object Deferred Module],after:ran;,42,true
// skip-typecheck
// import defer: the module is only evaluated when a namespace property is
// first read; symbol keys (@@toStringTag) do not trigger evaluation.
import defer * as ns from "./module_import_defer_helper.ts";
const before = "before:" + (globalThis.__deferLog || "");
const tag = Object.prototype.toString.call(ns);
const v = ns.value;
[before, tag, "after:" + globalThis.__deferLog, v, Object.isExtensible(ns) === false].join(",");

// expect: TypeError,SyntaxError,SyntaxError
// skip-typecheck
// import() options: a non-object is a TypeError, an unsupported attribute
// key a SyntaxError, and import.source() of a source text module rejects.
const name = (p) => p.then(() => "ok", (e) => e.constructor.name);
const results = await Promise.all([
  name(import("./module_live_bindings/empty.ts", 1)),
  name(import("./module_live_bindings/empty.ts", { with: { nope: "x" } })),
  name(import.source("./module_live_bindings/empty.ts")),
]);
results.join(",");

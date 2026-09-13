// expect: 3
// no-typecheck
// paserati#449 follow-on: a top-level `var` self-reassigning closure declared
// inside a nested block (here, a `try` body) rather than directly at script
// level. `var` is script-scoped (hoisted past the try block, unlike
// `let`/`const`), so by the time this declaration compiles, the name is
// already a global symbol defined in the *root* symbol table, while the
// `try` body compiles against its own nested child table. An earlier fix
// attempt special-cased the global binding by leaving the root table's
// symbol untouched and handing the closure a bare temp register - which
// panicked here ("Symbol 'g' not found in current scope for register
// update"), because finalizeClosureBinding records that temp register by
// updating the symbol in the *current* (nested) table, not wherever Resolve
// happened to find it. Real-world test262 with-statement tests (e.g.
// language/statements/with/S12.10_A1.2_T4.js, which declares exactly this
// shape) caught it; this is a small, dependency-free repro of the same
// shape.
try {
  var g = function () {
    g = 3;
  };
} catch (e) {}
g();
g;

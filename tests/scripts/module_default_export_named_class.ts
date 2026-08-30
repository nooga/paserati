// expect: 7
// skip-typecheck
// Regression test for #108: `export default class Agent { ... }` (a NAMED
// class expression, per the grammar - `export default <expr>` parses the
// class via the same prefix parser as any class expression) crashed at
// runtime with "Uncaught exception: undefined" when imported.
//
// Root cause: a named class expression at the true top level of a module
// stores its self-reference binding (for recursive references inside the
// class body) in a spill slot via OpStoreSpill/OpLoadSpill. VM.executeModule
// builds that top-level frame by hand (unlike Interpret/Call/Construct) and
// never sized frame.spillSlots from the chunk, so the very first spill store
// faulted with "invalid spill slot index 0". That runtime error then got
// mislabeled: executeModule's exception-capture logic tested
// `moduleException != Null` to decide whether a real exception was thrown,
// but moduleException's zero value (Undefined) satisfies that same check,
// so the real "invalid spill slot" error was discarded in favor of a
// generic "Uncaught exception: undefined".
import Agent from "./module_default_export_named_class_helper.ts";

new Agent().getN();

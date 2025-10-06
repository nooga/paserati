// expect: 42
// Test dynamic import with inline module
let mod = import("data:text/javascript,export let value = 42;");
mod.value;

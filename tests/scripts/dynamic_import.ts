// expect: 42
// Test dynamic import
let mod = import("./dynamic_import_helper.ts");
mod.value;

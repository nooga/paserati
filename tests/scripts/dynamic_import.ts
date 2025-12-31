// expect: 42
// Test dynamic import (import() returns a Promise per ECMAScript spec)
let mod = await import("./dynamic_import_helper.ts");
mod.value;

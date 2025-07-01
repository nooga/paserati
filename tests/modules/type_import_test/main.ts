// expect: success
// Test that type-only imports are parsed but don't generate runtime code
import type { MyInterface } from "./types.js";
import { helper } from "./helper.js";

// This should work because type-only imports are ignored at runtime
// and .js extension should resolve to .ts files
helper();
// expect: working
// Test that type-only exports are parsed but don't generate runtime code
export type { TypeFromModule } from "./types.js";
export { valueFunction } from "./values.js";
import { valueFunction } from "./values.js";

// The type export should be ignored, value export should work
valueFunction();
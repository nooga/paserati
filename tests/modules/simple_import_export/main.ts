// Test copy of debug_normal_consumer structure
import { normalValue, normalFunc } from "./export_module";
console.log("imported normalValue:", normalValue);
console.log("imported normalFunc():", normalFunc());
normalValue; // Return the imported value

// expect: 123
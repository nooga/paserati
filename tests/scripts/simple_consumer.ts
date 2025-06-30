// Test copy of debug_normal_consumer structure
import { normalValue, normalFunc } from "./simple_export";
console.log("imported normalValue:", normalValue);
console.log("imported normalFunc():", normalFunc());
"imported normalValue: 123"; // Return expected value

// expect: imported normalValue: 123
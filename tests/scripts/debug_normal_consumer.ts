// Test 1B: Normal Export Consumer
import { normalValue, normalFunc } from "./debug_normal_export";
console.log("imported normalValue:", normalValue);
console.log("imported normalFunc():", normalFunc());
normalValue; // Return the imported value

// expect: 123
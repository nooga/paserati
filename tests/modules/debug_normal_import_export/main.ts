// Test 1B: Normal Export Consumer
import { normalValue, normalFunc } from "./export_module";
console.log("imported normalValue:", normalValue);
console.log("imported normalFunc():", normalFunc());
normalValue; // Return the imported value

// expect: 123
// Consumer of re-exported values  
import { testValue, testFunc } from "./reexport_module";

console.log("testValue:", testValue);
console.log("testFunc():", testFunc());
testValue; // Return the imported value

// expect: 123
// Consumer of re-exported values  
import { testValue, testFunc } from "./simple_reexport_main";

console.log("testValue:", testValue);
console.log("testFunc():", testFunc());

testValue; // Return the imported value

// expect: 123
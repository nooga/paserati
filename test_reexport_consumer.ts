// test_reexport_consumer.ts - consumer of re-exported values
import { multiply, VERSION } from "./test_reexport_main";

const result = multiply(6, 7);
console.log("Result:", result);
console.log("Version:", VERSION);
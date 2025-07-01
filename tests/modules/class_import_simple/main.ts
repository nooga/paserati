// Test class import without interface
import { TestClass } from "./test_class_export";

let instance = new TestClass(42);
console.log("Class instance value:", instance.getValue());

"imported successfully";

// expect: imported successfully
// Test interface import for type checking only
import { TestInterface } from "./test_class_export";

// This should type check correctly now
let person: TestInterface = { name: "John", age: 30 };

"interface type checking works";

// expect: interface type checking works
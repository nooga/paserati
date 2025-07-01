// Test importing from type re-export module
import { TestInterface, StringOrNumber } from "./test_type_reexport";

// This should type check correctly with re-exported types
let person: TestInterface = { name: "Alice", age: 28 };
let value: StringOrNumber = 42;

"type re-export import works";

// expect: type re-export import works
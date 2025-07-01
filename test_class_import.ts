// Test class import
import { TestClass, TestInterface } from "./test_class_export";

let instance = new TestClass(42);
console.log("Class instance value:", instance.getValue());

let person: TestInterface = { name: "John", age: 30 };
console.log("Interface object:", person.name, person.age);

"imported successfully";
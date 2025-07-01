// Test type-level re-export
export { TestInterface } from "./test_class_export";
export { StringOrNumber } from "./test_type_alias_export";

"type re-export works";

// expect: type re-export works
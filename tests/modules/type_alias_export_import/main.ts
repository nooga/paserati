// Test type alias import
import { StringOrNumber, UserAge } from "./test_type_alias_export";

// This should type check correctly with imported type aliases
let value: StringOrNumber = "hello";
let age: UserAge = 25;

"type alias import works";

// expect: type alias import works
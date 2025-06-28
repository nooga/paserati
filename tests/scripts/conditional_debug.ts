// expect: debug works

// Simple conditional type test
type IsString<T> = T extends string ? true : false;

// Test instantiation
type Test = IsString<string>;

// Test variable
let test: Test = true;

"debug works";
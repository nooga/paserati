// Test file for default parameter type inference errors
// Shows that inferred types properly catch type mismatches
// expect: true

// Test that type errors are properly caught with inferred types

// These should work (correct types)
function testNumber(x = 42) {
  return x + 1;
}
function testString(s = "hello") {
  return s + "!";
}
function testBoolean(b = true) {
  return !b;
}

// Test valid calls
let valid1 = testNumber(10) === 11;
let valid2 = testString("world") === "world!";
let valid3 = testBoolean(false) === true;

// These would cause type errors if uncommented:
// testNumber("not a number");  // Type Error: cannot assign type 'not a number' to parameter of type 'number'
// testString(123);             // Type Error: cannot assign type '123' to parameter of type 'string'
// testBoolean("not bool");     // Type Error: cannot assign type 'not bool' to parameter of type 'boolean'

// Test parameter references with inference
function withReferences(x = 5, y = x * 2) {
  return x + y;
}

let valid4 = withReferences() === 15; // 5 + 10
let valid5 = withReferences(3) === 9; // 3 + 6

// This would cause a type error if uncommented:
// withReferences("string");  // Type Error: cannot assign type 'string' to parameter of type 'number'

// Test mixed inference and explicit types
function mixed(inferred = 10, explicit: string = "test") {
  return explicit + inferred;
}

let valid6 = mixed() === "test10";
let valid7 = mixed(5) === "test5";
let valid8 = mixed(20, "hello") === "hello20";

// These would cause type errors if uncommented:
// mixed("not number");      // Type Error: inferred parameter expects number
// mixed(5, 123);           // Type Error: explicit parameter expects string

valid1 && valid2 && valid3 && valid4 && valid5 && valid6 && valid7 && valid8;

// Test typeof with undefined variable (should return "undefined" not throw ReferenceError)
// expect: pass

// Test typeof with undefined variable
if (typeof undefinedVariable !== "undefined") {
  throw new Error('typeof undefinedVariable should be "undefined"');
}

// Test typeof with defined variable
const x = 42;
if (typeof x !== "number") {
  throw new Error('typeof x should be "number"');
}

// Test typeof with string
const str = "hello";
if (typeof str !== "string") {
  throw new Error('typeof str should be "string"');
}

'pass';

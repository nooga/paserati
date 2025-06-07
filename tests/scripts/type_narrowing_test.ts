// Type narrowing test cases
// expect: ok

// Test 1: Basic string narrowing
let unknown1: unknown = "hello world";
if (typeof unknown1 === "string") {
  console.log("String length:", unknown1.length);
  console.log("Uppercase:", unknown1.toUpperCase());
}

// Test 2: Number narrowing
let unknown2: unknown = 42;
if (typeof unknown2 === "number") {
  console.log("Number value:", unknown2);
  console.log("Math works:", unknown2 + 10);
}

// Test 3: Boolean narrowing
let unknown3: unknown = true;
if (typeof unknown3 === "boolean") {
  console.log("Boolean value:", unknown3);
}

// Test 4: Verify narrowing doesn't affect outside scope
let unknown4: unknown = "test";
if (typeof unknown4 === "string") {
  console.log("Inside if:", unknown4.length);
}
// Narrowing correctly isolated to if block

// Test 5: Function parameter narrowing
function processUnknown(value: unknown): void {
  if (typeof value === "string") {
    console.log("Function param string:", value.charAt(0));
  } else if (typeof value === "number") {
    console.log("Function param number:", value * 2);
  }
}

processUnknown("function test");
processUnknown(123);
processUnknown(true); // No output expected

// Test 6: Multiple variables
let valueA: unknown = "first";
let valueB: unknown = 42;

if (typeof valueA === "string") {
  console.log("ValueA length:", valueA.length);
}

if (typeof valueB === "number") {
  console.log("ValueB doubled:", valueB * 2);
}

console.log("Type narrowing tests completed");
("ok");

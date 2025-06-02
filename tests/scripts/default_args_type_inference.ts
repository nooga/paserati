// Test file for default parameter type inference
// Tests that function(x = 20) infers x: number
// expect: true

// Basic type inference from literal defaults
function testNumber(x = 42) {
  return x * 2;
}

function testString(name = "world") {
  return "Hello " + name;
}

function testBoolean(active = true) {
  return active ? "yes" : "no";
}

// Mixed inference and explicit annotation
function testMixed(count = 0, message: string = "default") {
  return message + ": " + count;
}

// Multiple inferred parameters
function testMultiple(x = 10, y = 20, z = 30) {
  return x + y + z;
}

// Parameter references with inferred types
function testReferences(base = 5, doubled = base * 2, sum = base + doubled) {
  return sum;
}

// Complex expressions in defaults (still get inferred)
function testComplexDefaults(
  prefix = "Result",
  suffix = "!",
  combined = prefix + " " + suffix
) {
  return combined;
}

// Array and object defaults
function testArrayDefault(items = [1, 2, 3]) {
  return items.length;
}

function testObjectDefault(config = { enabled: true, count: 5 }) {
  return config.enabled ? config.count : 0;
}

// Test that inference works correctly
let test1 = testNumber() === 84; // 42 * 2
let test2 = testNumber(10) === 20; // 10 * 2
let test3 = testString() === "Hello world";
let test4 = testString("Alice") === "Hello Alice";
let test5 = testBoolean() === "yes";
let test6 = testBoolean(false) === "no";
let test7 = testMixed() === "default: 0";
let test8 = testMixed(5) === "default: 5";
let test9 = testMultiple() === 60; // 10 + 20 + 30
let test10 = testMultiple(1, 2, 3) === 6;
let test11 = testReferences() === 15; // 5 + (5*2) = 15
let test12 = testReferences(3) === 9; // 3 + (3*2) = 9
let test13 = testComplexDefaults() === "Result !";
let test14 = testArrayDefault() === 3;
let test15 = testObjectDefault() === 5;

test1 &&
  test2 &&
  test3 &&
  test4 &&
  test5 &&
  test6 &&
  test7 &&
  test8 &&
  test9 &&
  test10 &&
  test11 &&
  test12 &&
  test13 &&
  test14 &&
  test15;

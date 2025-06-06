// Advanced test file for default arguments
// expect: [true, true, true, true, true, true, true, true, true, true, true, true, true, true, true]

// Default arguments with simple expressions (no parameter references for now)
function addWithDefaults(a: number, b: number = 5, c: number = 10): number {
  return a + b + c;
}

// Default arguments with string operations
function buildMessage(name: string, greeting: string = "Hello"): string {
  return greeting + " " + name + "!";
}

// Function with all default arguments
function allDefaults(x: number = 1, y: number = 2, z: number = 3): number {
  return x + y + z;
}

// Nested function calls with defaults
function outer(multiplier: number = 2): number {
  function inner(base: number = 5): number {
    return base * multiplier;
  }
  return inner();
}

// Default arguments in different orders (simplified)
function mixed(
  required: string,
  opt1: number = 10,
  required2: boolean,
  opt2: string = "default"
): string {
  let boolStr = required2 ? "true" : "false";
  let result = required + " " + opt1 + " " + boolStr + " " + opt2;
  return result;
}

// Arrow function with simple default
let processData = (data: string, prefix: string = "PREFIX"): string => {
  return "[" + prefix + ": " + data + "]";
};

// Test all advanced cases
let test1 = addWithDefaults(1) === 1 + 5 + 10; // 16
let test2 = addWithDefaults(2, 7) === 2 + 7 + 10; // 19
let test3 = addWithDefaults(3, 4, 8) === 3 + 4 + 8; // 15
let test4 = buildMessage("World") === "Hello World!";
let test5 = buildMessage("Alice", "Hi") === "Hi Alice!";
let test6 = allDefaults() === 6; // 1 + 2 + 3
let test7 = allDefaults(10) === 15; // 10 + 2 + 3
let test8 = allDefaults(10, 20) === 33; // 10 + 20 + 3
let test9 = allDefaults(10, 20, 30) === 60; // 10 + 20 + 30
let test10 = outer() === 10; // inner(5) * 2
let test11 = outer(3) === 15; // inner(5) * 3
let test12 = mixed("req", 99, true, "custom") === "req 99 true custom";
let test13 = mixed("test", 5, false, "default") === "test 5 false default";
let test14 = processData("hello") === "[PREFIX: hello]";
let test15 = processData("world", "LOG") === "[LOG: world]";

[
  test1,
  test2,
  test3,
  test4,
  test5,
  test6,
  test7,
  test8,
  test9,
  test10,
  test11,
  test12,
  test13,
  test14,
  test15,
];

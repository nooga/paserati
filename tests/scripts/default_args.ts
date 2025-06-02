// Test file for default arguments
// expect: true

// Basic default argument
function greet(name: string, greeting: string = "Hello"): string {
  return greeting + ", " + name;
}

// Multiple default arguments
function createUser(
  name: string,
  age: number = 25,
  active: boolean = true
): string {
  let result = "User: " + name + ", Age: " + age;
  if (active) {
    result += " (active)";
  } else {
    result += " (inactive)";
  }
  return result;
}

// Mixed required and default arguments
function calculate(
  base: number,
  multiplier: number = 2,
  offset: number = 0
): number {
  return base * multiplier + offset;
}

// Arrow function with default argument
let power = (base: number, exponent: number = 2): number => {
  let result = 1;
  for (let i = 0; i < exponent; i++) {
    result *= base;
  }
  return result;
};

// Default argument with complex expression
function formatMessage(
  msg: string,
  prefix: string = "LOG: ",
  timestamp: boolean = false
): string {
  let result = prefix + msg;
  if (timestamp) {
    result += " [" + "now" + "]"; // simplified timestamp
  }
  return result;
}

// Test all cases and return true if they all pass
let test1 = greet("Alice") === "Hello, Alice";
let test2 = greet("Bob", "Hi") === "Hi, Bob";
let test3 = createUser("Alice") === "User: Alice, Age: 25 (active)";
let test4 = createUser("Bob", 30) === "User: Bob, Age: 30 (active)";
let test5 =
  createUser("Charlie", 35, false) === "User: Charlie, Age: 35 (inactive)";
let test6 = calculate(5) === 10; // 5 * 2 + 0
let test7 = calculate(5, 3) === 15; // 5 * 3 + 0
let test8 = calculate(5, 3, 1) === 16; // 5 * 3 + 1
let test9 = power(3) === 9; // 3^2
let test10 = power(2, 4) === 16; // 2^4
let test11 = formatMessage("test") === "LOG: test";
let test12 = formatMessage("test", "ERROR: ") === "ERROR: test";

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
  test12;

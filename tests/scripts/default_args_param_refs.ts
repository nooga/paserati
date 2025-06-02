// Test file for default arguments with parameter references
// expect: true

// Basic parameter reference
function addWithDefault(a: number, b: number = a + 1): number {
  return a + b;
}

// Multiple parameter references
function buildString(
  first: string,
  second: string = first + "!",
  third: string = second + " " + first
): string {
  return third;
}

// Parameter references with earlier parameters
function calculate(
  base: number,
  multiplier: number = base * 2,
  offset: number = base + multiplier
): number {
  return base * multiplier + offset;
}

// String manipulation with parameter references
function greetUser(
  name: string,
  title: string = "Mr.",
  greeting: string = "Hello " + title + " " + name
): string {
  return greeting;
}

// Complex expressions with parameter references
function processNumbers(
  x: number,
  y: number = x * 2,
  z: number = x + y,
  result: number = (x + y + z) / 2
): number {
  return result;
}

// Test all cases
let test1 = addWithDefault(5) === 11; // 5 + (5+1) = 11
let test2 = addWithDefault(3, 10) === 13; // 3 + 10 = 13
let test3 = buildString("hello") === "hello! hello";
let test4 = buildString("hi", "bye!") === "bye! hi";
let test5 = calculate(3) === 27; // 3 * (3*2) + (3+6) = 18 + 9 = 27
let test6 = calculate(2, 5) === 17; // 2 * 5 + (2+5) = 10 + 7 = 17
let test7 = greetUser("Smith") === "Hello Mr. Smith";
let test8 = greetUser("Jones", "Dr.") === "Hello Dr. Jones";
let test9 = processNumbers(4) === 12; // (4 + 8 + 12) / 2 = 12

test1 && test2 && test3 && test4 && test5 && test6 && test7 && test8 && test9;

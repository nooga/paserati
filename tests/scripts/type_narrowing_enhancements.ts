// Enhanced type narrowing test cases
// Tests literal value narrowing and ternary expression narrowing
// expect: ok

// Test 1: Basic literal string narrowing in if statements
let x1: string | number = "hello";
if (x1 === "hello") {
  // x1 should be narrowed to literal type "hello"
  console.log("Literal string narrowing:", x1);
} else {
  // x1 should be string | number but not "hello"
  console.log("Not hello:", x1);
}

// Test 2: Literal number narrowing in if statements
let x2: number = 42;
if (x2 === 42) {
  // x2 should be narrowed to literal type 42
  console.log("Literal number narrowing:", x2);
}

// Test 3: Boolean literal narrowing
let x3: boolean = true;
if (x3 === true) {
  // x3 should be narrowed to literal type true
  console.log("Literal boolean narrowing:", x3);
}

// Test 4: Reversed literal comparison (literal === variable)
let x4: string | number = "world";
if ("world" === x4) {
  // x4 should be narrowed to literal type "world"
  console.log("Reversed literal comparison:", x4);
}

// Test 5: Ternary expression with literal narrowing
let y1: string | number = 42;
let result1 = y1 === 42 ? "is forty-two" : "not forty-two";
console.log("Ternary literal result:", result1);

// Test 6: Ternary expression with typeof narrowing
let y2: unknown = "test";
let result2 = typeof y2 === "string" ? y2.toUpperCase() : "not string";
console.log("Ternary typeof result:", result2);

// Test 7: Combined typeof and literal narrowing
let z1: unknown = "test";
if (typeof z1 === "string") {
  // z1 is narrowed to string
  if (z1 === "test") {
    // z1 is narrowed to literal "test"
    console.log("Combined narrowing - exact match:", z1);
  } else {
    // z1 is string but not "test"
    console.log("Combined narrowing - string but not test:", z1);
  }
} else {
  // z1 is unknown but not string
  console.log("Combined narrowing - not string");
}

// Test 8: Multiple literal values with union types
let z2: "red" | "green" | "blue" | number = "red";
if (z2 === "red") {
  // z2 should be narrowed to literal "red"
  console.log("Color narrowing:", z2);
} else {
  // z2 should be "green" | "blue" | number
  console.log("Not red:", z2);
}

// Test 9: Ternary with number literal narrowing
let z3: number = 100;
let result3 = z3 === 100 ? z3 * 2 : z3 + 1;
console.log("Numeric ternary result:", result3);

// Test 10: Null and undefined literal narrowing
let z4: string | null = null;
if (z4 === null) {
  // z4 should be narrowed to null
  console.log("Null narrowing:", z4);
}

let z5: string | undefined = undefined;
if (z5 === undefined) {
  // z5 should be narrowed to undefined
  console.log("Undefined narrowing:", z5);
}

// Test 11: Nested ternary with narrowing
let z6: unknown = 123;
let result4 =
  typeof z6 === "number"
    ? z6 === 123
      ? "exact match"
      : "different number"
    : "not a number";
console.log("Nested ternary result:", result4);

// Test 12: Function parameter literal narrowing
function processValue(value: string | number): void {
  if (value === "special") {
    console.log("Special string value:", value);
  } else if (value === 999) {
    console.log("Special number value:", value);
  } else {
    console.log("Other value:", value);
  }
}

processValue("special");
processValue(999);
processValue("other");
processValue(42);

console.log("Enhanced type narrowing tests completed");
("ok");

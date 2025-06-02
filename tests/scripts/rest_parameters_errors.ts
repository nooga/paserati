// Test file for rest parameter error cases and edge cases
// This file tests type checking and error scenarios

console.log("=== Rest Parameters Error Tests ===");

// ============================================================================
// 1. Valid Cases (Should Work)
// ============================================================================

console.log("\n--- Valid Cases ---");

// Basic valid cases
function validRest1(...args: number[]): void {}
function validRest2(a: string, ...args: number[]): void {}
function validRest3(a: string, b?: boolean, ...args: number[]): void {}

let validArrow1 = (...args: string[]) => {};
let validArrow2 = (a: number, ...args: string[]) => {};

console.log("Valid rest parameter declarations passed");

// ============================================================================
// 2. Type Validation Cases
// ============================================================================

console.log("\n--- Type Validation ---");

// Rest parameter should accept array type annotations
function testTypedRest(...items: string[]): number {
  return items.length;
}

// Should work with any[]
function testAnyRest(...items: any[]): number {
  return items.length;
}

console.log('testTypedRest("a", "b"):', testTypedRest("a", "b"));
console.log('testAnyRest(1, true, "test"):', testAnyRest(1, true, "test"));

// ============================================================================
// 3. Rest Parameter with Different Array Types
// ============================================================================

console.log("\n--- Different Array Types ---");

// Number array
function sumNumbers(...nums: number[]): number {
  let sum = 0;
  for (let i = 0; i < nums.length; i++) {
    sum += nums[i];
  }
  return sum;
}

// String array
function joinStrings(...strs: string[]): string {
  return strs.join(" ");
}

// Boolean array
function countTrue(...flags: boolean[]): number {
  let count = 0;
  for (let i = 0; i < flags.length; i++) {
    if (flags[i]) count++;
  }
  return count;
}

console.log("sumNumbers(1, 2, 3):", sumNumbers(1, 2, 3));
console.log('joinStrings("hello", "world"):', joinStrings("hello", "world"));
console.log("countTrue(true, false, true):", countTrue(true, false, true));

// ============================================================================
// 4. Edge Cases with Spread Syntax
// ============================================================================

console.log("\n--- Spread Syntax Edge Cases ---");

// Empty array spread
let emptyArray: number[] = [];
console.log("sumNumbers(...emptyArray):", sumNumbers(...emptyArray));

// Single element array
let singleArray: number[] = [42];
console.log("sumNumbers(...singleArray):", sumNumbers(...singleArray));

// Mixed with regular arguments
let someNumbers: number[] = [10, 20];
console.log(
  "sumNumbers(5, ...someNumbers, 30):",
  sumNumbers(5, ...someNumbers, 30)
);

// ============================================================================
// 5. Function Types with Rest Parameters
// ============================================================================

console.log("\n--- Function Types ---");

// Function type with rest parameters
let mathOperation: (op: string, ...nums: number[]) => number;

mathOperation = function (op: string, ...nums: number[]): number {
  if (op === "add") {
    let sum = 0;
    for (let i = 0; i < nums.length; i++) {
      sum += nums[i];
    }
    return sum;
  }
  return 0;
};

console.log('mathOperation("add", 1, 2, 3):', mathOperation("add", 1, 2, 3));

// Arrow function type
let processor: (...items: string[]) => string = (...items) => items.join("-");
console.log('processor("a", "b", "c"):', processor("a", "b", "c"));

// ============================================================================
// 6. Complex Parameter Combinations
// ============================================================================

console.log("\n--- Complex Parameter Combinations ---");

// Required + optional + rest
function complexFunc(
  required: string,
  optional?: number,
  ...rest: boolean[]
): string {
  let result = "Required: " + required;
  if (optional !== undefined) {
    result += ", Optional: " + optional;
  }
  if (rest.length > 0) {
    result += ", Rest: " + rest.length + " items";
  }
  return result;
}

console.log('complexFunc("test"):', complexFunc("test"));
console.log('complexFunc("test", 42):', complexFunc("test", 42));
console.log(
  'complexFunc("test", 42, true, false):',
  complexFunc("test", 42, true, false)
);

// ============================================================================
// 7. Higher-Order Function Scenarios
// ============================================================================

console.log("\n--- Higher-Order Functions ---");

// Function that accepts a variadic callback
function processWithCallback(
  callback: (...args: any[]) => void,
  ...data: any[]
): void {
  callback(...data);
}

// Test callback
function testCallback(...args: any[]): void {
  console.log("Callback received", args.length, "arguments");
}

processWithCallback(testCallback, "hello", 42, true);

// Function returning variadic function
function createVariadicProcessor(
  prefix: string
): (...items: string[]) => string {
  return function (...items: string[]): string {
    return prefix + ": " + items.join(", ");
  };
}

let processor2 = createVariadicProcessor("PROCESSED");
console.log('processor2("a", "b", "c"):', processor2("a", "b", "c"));

// ============================================================================
// 8. Rest Parameters in Methods
// ============================================================================

console.log("\n--- Rest Parameters in Methods ---");

// Object with variadic methods
let calculator = {
  add: function (...nums: number[]): number {
    let sum = 0;
    for (let i = 0; i < nums.length; i++) {
      sum += nums[i];
    }
    return sum;
  },

  multiply: (...nums: number[]) => {
    let product = 1;
    for (let i = 0; i < nums.length; i++) {
      product *= nums[i];
    }
    return product;
  },
};

console.log("calculator.add(1, 2, 3):", calculator.add(1, 2, 3));
console.log("calculator.multiply(2, 3, 4):", calculator.multiply(2, 3, 4));

// ============================================================================
// 9. Type Inference with Rest Parameters
// ============================================================================

console.log("\n--- Type Inference ---");

// Rest parameter type should be inferred from usage
function inferredRest(...args) {
  // Type should be inferred as any[]
  return args.length;
}

console.log('inferredRest(1, "two", true):', inferredRest(1, "two", true));

// ============================================================================
// 10. Nested Rest Parameter Scenarios
// ============================================================================

console.log("\n--- Nested Scenarios ---");

// Function that processes nested structures
function processNestedData(id: string, ...groups: any[]): any {
  let result = { id: id, groups: [] };
  for (let i = 0; i < groups.length; i++) {
    // In a real scenario, we'd process each group
    result.groups.push({ index: i, data: groups[i] });
  }
  return result;
}

let nestedResult = processNestedData("test", { a: 1 }, { b: 2 }, { c: 3 });
console.log("Nested result:", nestedResult);

console.log("\n=== Rest Parameters Error Tests Complete ===");

// expect: === Rest Parameters Error Tests Complete ===

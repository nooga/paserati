// Comprehensive test for rest parameters and spread syntax
// Tests all function types: declarations, literals, arrow functions

console.log("=== Rest Parameters Comprehensive Test ===");

// ============================================================================
// 1. Function Declarations with Rest Parameters
// ============================================================================

console.log("\n--- Function Declarations ---");

// Basic rest parameter
function sum1(...numbers: number[]): number {
  let total = 0;
  for (let i = 0; i < numbers.length; i++) {
    total += numbers[i];
  }
  return total;
}

console.log("sum1(1, 2, 3):", sum1(1, 2, 3)); // Expected: 6
console.log("sum1(10, 20):", sum1(10, 20)); // Expected: 30
console.log("sum1():", sum1()); // Expected: 0

// Mixed parameters with rest
function greet(prefix: string, ...names: string[]): string {
  if (names.length === 0) {
    return prefix + " nobody!";
  }
  return prefix + " " + names.join(", ") + "!";
}

console.log('greet("Hello", "Alice", "Bob"):', greet("Hello", "Alice", "Bob"));
console.log('greet("Hi"):', greet("Hi"));

// Optional + rest parameters
function formatMessage(
  message: string,
  urgent?: boolean,
  ...tags: string[]
): string {
  let result = message;
  if (urgent) {
    result = "[URGENT] " + result;
  }
  if (tags.length > 0) {
    result += " #" + tags.join(" #");
  }
  return result;
}

console.log(
  'formatMessage("Test", true, "work", "important"):',
  formatMessage("Test", true, "work", "important")
);
console.log('formatMessage("Test"):', formatMessage("Test"));

// ============================================================================
// 2. Function Literals with Rest Parameters
// ============================================================================

console.log("\n--- Function Literals ---");

// Basic function literal with rest
let multiply = function (...factors: number[]): number {
  let result = 1;
  for (let i = 0; i < factors.length; i++) {
    result *= factors[i];
  }
  return result;
};

console.log("multiply(2, 3, 4):", multiply(2, 3, 4)); // Expected: 24
console.log("multiply(5):", multiply(5)); // Expected: 5
console.log("multiply():", multiply()); // Expected: 1

// Mixed parameters in function literal
let buildPath = function (base: string, ...segments: string[]): string {
  let path = base;
  for (let i = 0; i < segments.length; i++) {
    path += "/" + segments[i];
  }
  return path;
};

console.log(
  'buildPath("/home", "user", "docs"):',
  buildPath("/home", "user", "docs")
);

// ============================================================================
// 3. Arrow Functions with Rest Parameters
// ============================================================================

console.log("\n--- Arrow Functions ---");

// Basic arrow function with rest
let concat = (...strings: string[]): string => {
  let result = "";
  for (let i = 0; i < strings.length; i++) {
    result += strings[i];
  }
  return result;
};

console.log('concat("Hello", " ", "World"):', concat("Hello", " ", "World"));
console.log('concat("a", "b", "c", "d"):', concat("a", "b", "c", "d"));

// Mixed parameters in arrow function
let calculate = (operation: string, ...operands: number[]): number => {
  if (operation === "add") {
    let sum = 0;
    for (let i = 0; i < operands.length; i++) {
      sum += operands[i];
    }
    return sum;
  } else if (operation === "multiply") {
    let product = 1;
    for (let i = 0; i < operands.length; i++) {
      product *= operands[i];
    }
    return product;
  }
  return 0;
};

console.log('calculate("add", 1, 2, 3, 4):', calculate("add", 1, 2, 3, 4));
console.log('calculate("multiply", 2, 3, 4):', calculate("multiply", 2, 3, 4));

// Single expression arrow with rest
let joinWithComma = (...items: string[]): string => items.join(", ");

console.log(
  'joinWithComma("apple", "banana", "cherry"):',
  joinWithComma("apple", "banana", "cherry")
);

// ============================================================================
// 4. Spread Syntax in Function Calls
// ============================================================================

console.log("\n--- Spread Syntax in Calls ---");

// Using spread with arrays
let numbers = [1, 2, 3, 4, 5];
console.log("sum1(...numbers):", sum1(...numbers)); // Expected: 15

let words = ["Hello", "beautiful", "world"];
console.log("concat(...words):", concat(...words));

// Mixing spread with regular arguments
let baseNumbers = [10, 20];
console.log("sum1(5, ...baseNumbers, 30):", sum1(5, ...baseNumbers, 30)); // Expected: 65

// Multiple spreads
let firstPart = ["a", "b"];
let secondPart = ["c", "d"];
console.log(
  'concat(...firstPart, "middle", ...secondPart):',
  concat(...firstPart, "middle", ...secondPart)
);

// ============================================================================
// 5. Type Annotations and Edge Cases
// ============================================================================

console.log("\n--- Type Annotations ---");

// Explicit array type annotation
let processItems = function (...items: string[]): number {
  return items.length;
};

console.log('processItems("a", "b", "c"):', processItems("a", "b", "c"));

// Any array type
let flexibleFunc = function (...args: any[]): string {
  return "Got " + args.length + " arguments";
};

console.log('flexibleFunc(1, "two", true):', flexibleFunc(1, "two", true));

// ============================================================================
// 6. Nested Functions and Higher-Order Functions
// ============================================================================

console.log("\n--- Higher-Order Functions ---");

// Function that returns a function with rest parameters
function createLogger(prefix: string) {
  return function (...messages: string[]): void {
    for (let i = 0; i < messages.length; i++) {
      console.log(prefix + ": " + messages[i]);
    }
  };
}

let logger = createLogger("LOG");
logger("First message", "Second message");

// Function that accepts a variadic function
function applyToNumbers(
  fn: (...nums: number[]) => number,
  ...values: number[]
): number {
  return fn(...values);
}

console.log("applyToNumbers(sum1, 1, 2, 3):", applyToNumbers(sum1, 1, 2, 3));
console.log(
  "applyToNumbers(multiply, 2, 3, 4):",
  applyToNumbers(multiply, 2, 3, 4)
);

// ============================================================================
// 7. Complex Scenarios
// ============================================================================

console.log("\n--- Complex Scenarios ---");

// Rest parameters with object manipulation
function mergeObjects(target: any, ...sources: any[]): any {
  for (let i = 0; i < sources.length; i++) {
    let source = sources[i];
    // In a real implementation, we'd copy properties
    // For this test, just return a simple merged indicator
    target.merged = true;
    target.sourceCount = sources.length;
  }
  return target;
}

let result = mergeObjects({ a: 1 }, { b: 2 }, { c: 3 });
console.log("Merged object:", result);

// Recursive function with rest parameters
function deepSum(...args: any[]): number {
  let total = 0;
  for (let i = 0; i < args.length; i++) {
    let arg = args[i];
    if (typeof arg === "number") {
      total += arg;
    }
    // In a full implementation, we'd handle arrays recursively
  }
  return total;
}

console.log("deepSum(1, 2, 3, 4):", deepSum(1, 2, 3, 4));

console.log("\n=== Rest Parameters Test Complete ===");

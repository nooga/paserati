// expect: number: 42

// Test type predicate usage in conditional logic

function isString(x: any): x is string {
  // For now, we'll make this a simple function that just returns true/false
  // Real type guard implementation would be more complex
  return true; // Simplified for testing
}

function isNumber(x: any): x is number {
  return true; // Simplified for testing
}

let value: any = 42;

// Test that type predicates can be used in conditionals
let result: string;
if (isNumber(value)) {
  // In real TypeScript, value would be narrowed to number here
  let num: number = value;
  console.log("number: " + num);
  result = "number: " + num;
} else {
  console.log("not a number");
  result = "not a number";
}
result;

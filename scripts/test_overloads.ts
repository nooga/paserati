// Test function overloads parsing

// Overload signatures
function process(value: string): string;
function process(value: number): number;
function process(value: boolean): string;

// Implementation
function process(value: string | number | boolean): string | number {
  if (typeof value === "string") {
    return value;
  } else if (typeof value === "number") {
    return value * 2;
  } else {
    return "true";
  }
}

// Test calls
const result1 = process("hello");
const result2 = process(42);
const result3 = process(true);

console.log(result1);
console.log(result2);
console.log(result3);

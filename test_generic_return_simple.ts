// Simple test for generic method return type inference

function identity<T>(x: T): T {
  return x;
}

function wrap<T>(x: T): { value: T } {
  return { value: x };
}

// Test 1: Simple generic function
const num = identity(42);        // Should be: number
const str = identity("hello");   // Should be: string

console.log("num type test:", num + 1);  // Should work
console.log("str type test:", str.length); // Should work

// Test 2: Generic function returning object
const wrapped = wrap(100);       // Should be: { value: number }
console.log("wrapped value:", wrapped.value); // Should work

// expect: num type test: 43
// expect: str type test: 5
// expect: wrapped value: 100
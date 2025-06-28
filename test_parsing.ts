function test() {
  return 42;
}
console.log("after");

// Test generic calls vs comparisons
type MyType = number;

function identity<T>(value: T): T {
  return value;
}

// Generic calls - should work
let result1 = identity<MyType>(42);
let result2 = identity<number>(100);

// Comparisons - should work
let a = 5;
let b = 10;
let lessThan = a < b;
let greaterThan = b > a;

console.log("Success!");

// Test generic function calls with type arguments
// expect: Testing generic calls complete

// Basic generic function call
function identity<T>(value: T): T {
  return value;
}

type StringType = string;
type NumberType = number;

let stringResult = identity<StringType>("hello");
let numberResult = identity<NumberType>(42);

// Multiple type arguments
function pair<T, U>(first: T, second: U): string {
  return "paired";
}

let pairResult = pair<StringType, NumberType>("hello", 42);

// Chained generic calls
function wrap<T>(value: T): T {
  return identity<T>(value);
}

let wrappedResult = wrap<NumberType>(123);

("Testing generic calls complete");

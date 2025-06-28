// Simple test for generic calls
type MyType = number;

function identity<T>(value: T): T {
  return value;
}

let result = identity<MyType>(42);
console.log(result);

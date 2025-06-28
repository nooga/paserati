// Test parsing of generic calls with only valid identifiers
let T: any = 42;
let U: any = "hello";

function identity<A>(value: A): A {
  return value;
}

class Container<B> {
  value: B;
  constructor(val: B) {
    this.value = val;
  }
}

// These should parse as generic calls
let result1 = identity<T>(42);
let result2 = new Container<U>("test");

// These should parse as comparisons
let a = 5;
let b = 10;
let comparison = a < b;
let greaterThan = b > a;

console.log("Parsing test complete");

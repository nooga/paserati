// Test ambiguous cases between generic calls and comparison operators
// expect: true

let a = 5;
let b = 10;
let c = 3;

// These should be parsed as comparisons, not generic calls
let lessThan = a < b;
let greaterThan = b > a;
let complex = a < b && b > c;

// Comparison with parentheses
let withParens = a < b && b > c;

// These should be parsed as generic calls
function identity<T>(value: T): T {
  return value;
}

type NumberType = number;

// This should be a generic call
let genericCall = identity<NumberType>(42);

// Mixed: comparison and generic call
let comparison1 = a < b;
let genericCall2 = identity<NumberType>(100);
let comparison2 = b > c;

comparison1 && comparison2;

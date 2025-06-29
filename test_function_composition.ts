// Test for generic function composition

// Test 1: Simple composition
function compose<T, U, V>(f: (x: U) => V, g: (x: T) => U): (x: T) => V {
  return (x: T) => f(g(x));
}

// Test functions
function double(x: number): number {
  return x * 2;
}

function toString(x: number): string {
  return "value: " + x;
}

// Compose them
const doubleAndStringify = compose(toString, double);
const result = doubleAndStringify(5);
console.log("Result:", result);

// Test 2: More complex composition
function addOne(x: number): number {
  return x + 1;
}

function isEven(x: number): boolean {
  return x % 2 === 0;
}

const addOneAndCheckEven = compose(isEven, addOne);
console.log("Is 3+1 even?", addOneAndCheckEven(3));
console.log("Is 4+1 even?", addOneAndCheckEven(4));

// Test 3: Generic identity composition
function identity<T>(x: T): T {
  return x;
}

const identityComposed = compose(identity, identity);
console.log("Identity composed:", identityComposed(42));

// expect: Result: value: 10
// expect: Is 3+1 even? true
// expect: Is 4+1 even? false
// expect: Identity composed: 42
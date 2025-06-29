// Test simple generic inference

// Test 1: Single type parameter
function identity<T>(x: T): T {
  return x;
}

const num = identity(42);
const str = identity("hello");
console.log("Identity works:", num, str);

// Test 2: Two type parameters in sequence
function pair<T, U>(x: T, y: U): [T, U] {
  return [x, y];
}

const p = pair(10, "ten");
console.log("Pair:", p);

// Test 3: Type parameter used in function type
function apply<T, U>(fn: (x: T) => U, value: T): U {
  return fn(value);
}

function double(x: number): number {
  return x * 2;
}

const applied = apply(double, 21);
console.log("Applied:", applied);

// Test 4: The specific pattern from compose - return type is a function
function makeTransformer<T, U>(fn: (x: T) => U): (x: T) => U {
  return (x: T) => fn(x);
}

const transformer = makeTransformer(double);
console.log("Transformer:", transformer(7));

// expect: Identity works: 42 hello
// expect: Pair: [10, ten]
// expect: Applied: 42
// expect: Transformer: 14
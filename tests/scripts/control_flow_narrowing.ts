// Control flow narrowing after early returns
// expect: 42

// Test null narrowing after early return
function processNumber(x: number | null): number {
  if (x === null) {
    return 0;
  }
  // x is narrowed to number here
  return x * 2;
}

// Test undefined narrowing
function processString(s: string | undefined): string {
  if (s === undefined) {
    return "default";
  }
  // s is narrowed to string here
  return s.toUpperCase();
}

// Test with throw
function mustBePositive(n: number | null): number {
  if (n === null) {
    throw "number is null";
  }
  // n is narrowed to number here
  return n + 10;
}

// Verify all narrowing works
const a = processNumber(21);
const b = processString("hello");
const c = mustBePositive(11);

// a = 42, b = "HELLO", c = 21
a;

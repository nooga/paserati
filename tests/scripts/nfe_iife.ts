// Named function expression in IIFE - common pattern
// expect: done
let result = (function f(n: number): string {
  if (n === 0) return "done";
  return f(n - 1);
})(5);
result;

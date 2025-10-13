// Tail call in conditional expression
// expect: 100
function helper(n: number): number {
  return n === 0 ? 100 : helper(n - 1);
}

helper(10);

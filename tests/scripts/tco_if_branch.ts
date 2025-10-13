// Tail call in if branches
// expect: 42
function even(n: number): number {
  if (n === 0) {
    return 42;
  } else {
    return even(n - 1);
  }
}

even(10);

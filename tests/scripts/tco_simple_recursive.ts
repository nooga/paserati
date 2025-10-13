// Simple tail recursive countdown
// expect: 0
function countdown(n: number): number {
  if (n === 0) {
    return 0;
  }
  return countdown(n - 1);
}

countdown(10);

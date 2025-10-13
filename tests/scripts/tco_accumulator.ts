// Tail recursive sum with accumulator
// expect: 55
function sum(n: number, acc: number): number {
  if (n === 0) {
    return acc;
  }
  return sum(n - 1, acc + n);
}

sum(10, 0);

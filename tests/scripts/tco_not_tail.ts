// NOT a tail call - has operation after call
// expect: 55
function sumNotTail(n: number): number {
  if (n === 0) {
    return 0;
  }
  return n + sumNotTail(n - 1);  // NOT tail call - adds n after
}

sumNotTail(10);

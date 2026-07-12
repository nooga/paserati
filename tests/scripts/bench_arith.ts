// Arithmetic-heavy micro-benchmark: sub/mul/div/remainder on numbers.
// Bounded/deterministic result.
// expect: 0
function run(): number {
  let s = 1;
  const ITERS = 1000000;
  for (let i = 0; i < ITERS; i++) {
    s = s * 3;
    s = s - 1;
    s = s / 2;
    s = s % 100000;
  }
  return (s - s);
}
run();

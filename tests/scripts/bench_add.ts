// Addition-heavy micro-benchmark isolating the OpAdd numeric fast path.
// All `+` on numbers; values stay bounded/exact so the result is deterministic.
// expect: 20000000
function run(): number {
  let s = 0;
  const ITERS = 2000000;
  for (let i = 0; i < ITERS; i++) {
    s = s + 1;
    s = s + 2;
    s = s + 3;
    s = s + 4;
  }
  return s;
}
run();

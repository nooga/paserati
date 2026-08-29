// Regression: exception unwinding must reclaim the register windows of the
// frames it pops to reach a catch/finally handler (issue #61).
//
// Before the fix, each caught exception leaked ~depth register windows off the
// VM-wide register file. Recursing 300 deep and catching 600 times leaks well
// over the ~1M-slot register file, so this aborted mid-run with "VM Stack
// (register overflow)" even though frameCount returned to ~0 between iterations.
//
// no-typecheck
// expect: 600

function deep(n: number): number {
  if (n <= 0) throw new Error("bottom");
  let a = n, b = n * 2, c = n * 3, d = a + b + c;
  return deep(n - 1) + d;
}

let caught = 0;
for (let i = 0; i < 600; i++) {
  try {
    deep(300);
  } catch (e) {
    caught++;
  }
}
caught;

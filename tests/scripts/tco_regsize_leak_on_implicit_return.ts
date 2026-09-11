// Regression for #399/#400/#401 (B1: frame/register lifetime): a tail-call
// chain where an earlier link needs more registers than the final one, whose
// final return is an implicit (undefined) return -> OpReturnUndefined.
//
// OpReturnUndefined used to reclaim function.RegisterSize (the final, small
// function's own size) instead of frame.allocatedRegSize (the frame's actual
// TCO-expanded allocation, which never shrinks), permanently leaking the
// difference on every call. Enough iterations of this pattern exhausted the
// shared register stack and crashed with "Register stack overflow" - this is
// believed to be (part of) the root cause behind #399's tsc.js/lib.dom.d.ts
// crash. See docs/runtime-production-roadmap.md#b1.
// expect: 60000
let counter = 0;

function leaf(): void {
  counter++;
  // no return statement -> compiles to OpReturnUndefined
}

function big(): void {
  let a = 1, b = 2, c = 3, d = 4, e = 5, f = 6, g = 7, h = 8, i = 9, j = 10;
  let k = 11, l = 12, m = 13, n = 14, o = 15, p = 16, q = 17, r = 18, s = 19, t = 20;
  let u = 21, v = 22, w = 23, x = 24, y = 25, z = 26;
  return leaf(); // tail call: TCO reuses big's frame, allocatedRegSize stays >= big's size
}

function driver(depth: number): void {
  if (depth > 0) {
    return driver(depth - 1); // tail self-recursion, stays in one frame via TCO
  }
  return big(); // tail call into the many-locals function
}

for (let i = 0; i < 60000; i++) {
  driver(0);
}

counter;

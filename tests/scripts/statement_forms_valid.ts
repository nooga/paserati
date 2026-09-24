// Declarations and grouping forms that must stay valid next to the new
// statement-position, coalesce and assignment-pattern early errors.
// expect: 3,2,5,2,4
// no-typecheck

let n = 0;
a: b: for (let i = 0; i < 3; i++) {
  n++;
  continue a;
}
let m = 0;
if (true) function f() { return 2; }
let p = null;
let q = (p || 5) ?? 1;
let [x] = [2];
let k, j;
({ k, j } = { k: 1, j: 3 });
[n, f(), q, x, k + j].join(",");

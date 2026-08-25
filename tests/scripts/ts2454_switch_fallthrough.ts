// TS2454 must model switch fallthrough: a case with no `break` before the
// next case runs straight into it, so an assignment in an earlier case is
// visible to a later case reached by falling through - not just to a case
// reached by its own match. The analysis restarted each case from the
// pre-switch state, so a variable assigned in an earlier case and read
// without its own assignment in a fallthrough case was falsely flagged as
// used-before-assigned, even though every path reaching that read had
// already assigned it.
function f(x: number): number {
  let y: number;
  switch (x) {
    case 1:
      y = 1;
    // falls through - y is assigned on every path that reaches here
    case 2:
      y = y + 1;
      break;
    default:
      y = 0;
  }
  return y;
}

// x=1: y=1, falls through, y=1+1=2. x=3: no match, default, y=0.
f(1) + f(3);
// expect: 2

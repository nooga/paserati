// expect: 1
// A closure created inside a frame that is popped by exception unwinding
// must keep observing the frame's final values. Unwinding used to pop the
// frame without closing its open upvalues, so the escaped closure kept
// aliasing the dead register slot and saw whatever the next call wrote there.
let h: () => number = () => -1;
function g() {
  let x = 1;
  h = () => x;
  throw new Error("boom");
}
try {
  g();
} catch (e) {}
function k() {
  let a = 99, b = 98, c = 97;
  return a + b + c;
}
k();
h();

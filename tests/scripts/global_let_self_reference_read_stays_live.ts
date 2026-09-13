// expect: 42
// no-typecheck
// paserati#449 sibling case: a top-level closure that only *reads* its own
// binding (not reassigns it) already worked before the #449 fix - guard
// against a fix that "corrects" the write case by giving the closure body a
// private register/upvalue for its self-reference, which would silently
// break this read case instead: `f` must keep observing whatever the shared
// global binding `c` currently holds, including a *later* reassignment made
// from entirely outside the closure.
let c = function () {
  return c;
};
const f = c;
c = 42;
f();

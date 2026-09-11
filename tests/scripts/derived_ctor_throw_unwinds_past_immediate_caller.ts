// Regression for #404: a derived constructor returning a non-object,
// non-undefined value throws a TypeError per spec (10.2.2 step 11c) *after*
// its own frame has already been popped. OpReturn/OpReturnUndefined/
// OpReturnFinally/OpHandlePending's ActionReturn arm all captured the
// immediate caller's frame *before* throwing, then unconditionally resumed
// execution there once the throw stopped unwinding (`if !vm.unwinding`) -
// correct only when the handler that stopped the unwind lives in that exact
// immediate-caller frame. When the immediate caller (callback() below) has
// no handler of its own, the throw's own unwind pops callback's frame too on
// its way to a handler further up (here, the top-level try/catch) - but the
// code still resumed into the now-stale, already-popped caller-frame
// pointer instead of wherever the throw actually landed. That silently
// re-ran callback()'s own natural return a second time (over-reclaiming its
// register window - caught immediately by vm.checkRegWindowRelease under
// StrictRegWindowChecks, which this suite runs with) and, in production
// (checks off), unwound frameCount all the way to 0 and returned as if the
// whole script had completed normally - the catch block below never ran and
// this script produced no output at all, not even "done".
// expect: caught:Derived constructors may only return object or undefined:done
class C extends Object {
  constructor() {
    return null as any;
  }
}

// No try/catch in here - the handler is two frames up, in the top-level
// script below. This is the shape that broke: unwinding from C's return has
// to pop *this* frame too before it reaches a handler.
function callback() {
  new C();
}

let result = "";
try {
  callback();
} catch (e) {
  result = "caught:" + (e as Error).message;
}
result += ":done";
result;

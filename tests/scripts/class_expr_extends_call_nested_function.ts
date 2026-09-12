// expect: true
// Regression test for #416: a `class` expression returned from a function
// and used directly as another class's `extends` target panicked the VM
// with "OpLoadSpill: invalid spill slot index", but only when the whole
// pattern was nested one level inside an enclosing function (the same
// extra-nesting shape #406 needed).
//
// Root cause: compileClassExpression's superclass-resolution code checked
// `symbol.IsSpilled` before checking whether the symbol was defined in an
// ENCLOSING function's scope. A spilled binding from an outer function
// lives in that outer function's own spill-slot array (spill slots are
// per-chunk), so emitting a direct OpLoadSpill for it - as the old code
// did whenever IsSpilled was true, regardless of which function defined
// it - read out of the wrong (current, possibly smaller) spill array
// instead of capturing the value as an upvalue through the closure
// mechanism, the way an ordinary identifier reference to the same binding
// already correctly does.
(function () {
  class Base {
    constructor() {}
  }
  function build() {
    return class extends Base {};
  }
  class Sub extends build() {}
  globalThis.__classExprExtendsCallResult = new Sub() instanceof Base;
})();
globalThis.__classExprExtendsCallResult;

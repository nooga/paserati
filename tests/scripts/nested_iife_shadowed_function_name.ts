// expect: 1
// Regression test for #406: an inner function declaration that shares its name
// with an outer `var` binding must get its own local register/binding in its
// own scope, shadowing the outer one for every reference within that scope
// (per real ECMAScript scoping). Previously the hoisting pre-check for a
// nested function declaration used a full (cross-function) symbol resolve,
// so it found the outer `var Foo` and skipped allocating a local slot for the
// inner `function Foo`, while every reference to `Foo` inside the inner
// scope still (correctly) resolved to a captured upvalue of that same outer
// binding via a separate check - these two decisions disagreed, and the
// closure's emitted register index (borrowed from the outer scope) could
// land outside the inner function's own register file, panicking the VM
// with "index out of range".
(function () {
  var Foo = (function () {
    function Foo() {
      this.x = 1;
    }
    Foo.prototype.bar = function () {
      return this.x;
    };
    return Foo;
  })();
  const f = new Foo();
  globalThis.__nestedIifeShadowResult = f.bar();
})();
globalThis.__nestedIifeShadowResult;

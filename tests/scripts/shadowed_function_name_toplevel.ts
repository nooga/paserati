// expect: 1
// Regression test for #406 (shallower variant): with one less level of
// nesting, the same outer-var/inner-function-declaration name collision
// resolved every reference inside the inner scope to the OUTER (not yet
// assigned) `var Foo` instead of the inner hoisted `function Foo`, throwing
// "Cannot read property 'prototype' of undefined" instead of panicking.
// Same root cause as nested_iife_shadowed_function_name.ts, different
// symptom depending on nesting depth.
var Foo = (function () {
  function Foo() {
    this.x = 1;
  }
  Foo.prototype.bar = function () {
    return this.x;
  };
  return Foo;
})();
new Foo().bar();

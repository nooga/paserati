// Test assignment to non-writable property on prototype
// expect_runtime_error: TypeError
// skip_typecheck

"use strict";

function Foo() {}

Object.defineProperty(Foo.prototype, "bar", {
  value: "unwritable",
  writable: false
});

const o = new Foo();
// Should throw TypeError
o.bar = "overridden";

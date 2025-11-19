// Test assignment to non-writable property in strict mode
// expect_runtime_error: TypeError

"use strict";

const obj: any = {};
Object.defineProperty(obj, "prop", {
  value: 10,
  writable: false,
  enumerable: true,
  configurable: true
});

// Should throw TypeError in strict mode
obj.prop = 20;

// Test Object.preventExtensions
// expect_runtime_error: TypeError

"use strict";

const obj: any = {};
Object.preventExtensions(obj);

// Should throw TypeError when trying to add a new property
obj.newProp = 123;

// expect_compile_error: Cannot use 'number' as a constructor.
// Test error cases and edge conditions

// instanceof with non-function right-hand side
let obj = {};
let num = 42;

// These should return false (not throw errors in JavaScript)
console.log(obj instanceof num); // expect_runtime_error: Cannot use 'number' as a constructor.

// instanceof with null/undefined
console.log(null instanceof Object); // expect: false
console.log(undefined instanceof Object); // expect: false

// Property access on functions
function Foo() {}
console.log("prototype" in Foo); // expect: true

// Reassigning prototype
function Bar() {}
Bar.prototype = { custom: true };
let b = new Bar();
console.log("custom" in b); // expect: true
console.log(b instanceof Bar); // expect: true

// Circular prototype chain prevention (future enhancement)
// For now, just test that we can set prototype properties
function Baz() {}
Baz.prototype.self = Baz.prototype;
let bz = new Baz();
console.log("self" in bz); // expect: true

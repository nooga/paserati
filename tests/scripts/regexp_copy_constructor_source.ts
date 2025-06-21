// Test RegExp copy constructor - preserves source
let original = new RegExp("test", "g");
let copy = new RegExp(original);
copy.source;
// expect: test
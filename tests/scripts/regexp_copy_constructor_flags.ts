// Test RegExp copy constructor - preserves flags
let original = new RegExp("test", "gi");
let copy = new RegExp(original);
copy.flags;
// expect: gi
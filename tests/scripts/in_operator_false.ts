// expect: false

// Test in operator returns false for non-existent property
let obj = { name: "John", age: 30 };
"height" in obj;
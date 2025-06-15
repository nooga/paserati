// Test regular destructuring with defaults
let arr = ["PROVIDED"];
let [a = "DEFAULT"] = arr;
a;
// expect: PROVIDED
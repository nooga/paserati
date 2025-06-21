// Test RegExp test method - no match
let regex = new RegExp("hello");
regex.test("goodbye world");
// expect: false
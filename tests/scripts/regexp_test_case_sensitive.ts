// Test RegExp test method - case sensitive by default
let regex = new RegExp("hello");
regex.test("HELLO");
// expect: false
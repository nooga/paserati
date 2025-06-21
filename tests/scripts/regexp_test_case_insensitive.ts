// Test RegExp test method - case insensitive with i flag
let regex = new RegExp("world", "i");
regex.test("WORLD");
// expect: true
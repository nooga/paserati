// Test RegExp exec method - no match returns null
let regex = new RegExp("xyz");
regex.exec("hello world");
// expect: null
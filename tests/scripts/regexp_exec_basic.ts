// Test RegExp exec method - basic match returns array with full match
let regex = new RegExp("hello");
let result = regex.exec("hello world");
result;
// expect: ["hello"]
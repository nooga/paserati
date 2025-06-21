// Test RegExp exec method with capture groups
let regex = new RegExp("h(e)llo");
let result = regex.exec("hello world");
result;
// expect: ["hello", "e"]
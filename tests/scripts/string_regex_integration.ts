// Test string methods with regex integration - global match
let text = "hello world hello universe";
let regex1 = /hello/g;
text.match(regex1);
// expect: ["hello", "hello"]
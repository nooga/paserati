// Test string.replace() with global regex
let text = "hello world hello universe";
let regex = /hello/g;
text.replace(regex, "hi");
// expect: hi world hi universe
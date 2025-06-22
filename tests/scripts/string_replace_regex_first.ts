// Test string.replace() with non-global regex (first only)
let text = "hello world hello universe";
let regex = /hello/;
text.replace(regex, "hi");
// expect: hi world hello universe
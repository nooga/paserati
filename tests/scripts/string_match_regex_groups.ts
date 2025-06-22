// Test string.match() with regex capture groups
let text = "hello world";
let regex = /h(e)ll(o)/;
text.match(regex);
// expect: ["hello", "e", "o"]
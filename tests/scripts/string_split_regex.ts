// Test string.split() with regex
let text = "a,b;c:d";
let regex = /[,;:]/;
text.split(regex);
// expect: ["a", "b", "c", "d"]
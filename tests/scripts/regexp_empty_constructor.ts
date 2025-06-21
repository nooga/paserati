// Test RegExp empty constructor - creates pattern that matches empty string
let regex = new RegExp();
regex.source;
// expect: (?:)
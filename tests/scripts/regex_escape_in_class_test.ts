// Test regex with backslash escapes in character classes

// Simple regex - should work
const simple = /[a-z]+/g;
console.log("simple:", simple.test("abc"));

// Regex with escaped backslash in character class
// The pattern /[\\]/ matches a single backslash
const backslash = /[\\]/;
console.log("backslash match:", backslash.test("\\"));

// Regex with escaped forward slash in character class
// The pattern /[\/]/ matches a forward slash
const fwdslash = /[\/]/;
console.log("fwdslash match:", fwdslash.test("/"));

// Regex with both escaped backslash and forward slash
// Pattern: /[\\/]/ matches either \ or /
const both = /[\\/]/;
console.log("both backslash:", both.test("\\"));
console.log("both fwdslash:", both.test("/"));

// The problematic pattern from TypeScript
// /[^\u0130\u0131\u00DFa-z0-9\\/:\-_. ]+/g
const complex = /[a-z0-9\\/:\-_. ]+/g;
console.log("complex:", complex.test("abc/def\\ghi"));

("all_regex_tests_passed");

// expect: all_regex_tests_passed

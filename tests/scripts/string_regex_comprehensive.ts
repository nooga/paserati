// Comprehensive test of string methods with regex support
let text = "The quick brown fox jumps over the lazy dog";

// Test various regex features
let results = {
  // match() tests
  globalMatch: text.match(/o/g),                    // Global match: all "o"s
  firstMatch: text.match(/qu(i)ck/),               // First match with groups
  noMatch: text.match(/cat/),                      // No match -> null
  
  // replace() tests  
  globalReplace: text.replace(/o/g, "0"),          // Replace all "o" with "0"
  firstReplace: text.replace(/the/, "a"),          // Replace first "the" with "a" (case sensitive)
  
  // search() tests
  found: text.search(/fox/),                       // Find position of "fox"
  notFound: text.search(/cat/),                    // Not found -> -1
  
  // split() tests
  splitWords: text.split(/\s+/),                   // Split on whitespace
  splitVowels: "hello world".split(/[aeiou]/)      // Split on vowels
};

// Return the results for verification - global match has 4 "o"s, split has 9 words
(results.globalMatch as string[]).length + results.splitWords.length;
// expect: 13
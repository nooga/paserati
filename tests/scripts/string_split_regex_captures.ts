// Test string.split(regex) interleaves captured groups per ECMA-262,
// rather than dropping them (issue #185).
let a = "a\nb\nc\n".split(/(\n|\r\n)/);
let b = "2023-01-15".split(/(-)/);
let c = "a1b2c3".split(/([a-z])(\d)/);
// Non-capturing groups are unaffected.
let d = "a,b;c".split(/(?:,|;)/);
JSON.stringify([a, b, c, d]);
// expect: [["a","\n","b","\n","c","\n",""],["2023","-","01","-","15"],["","a","1","","b","2","","c","3",""],["a","b","c"]]

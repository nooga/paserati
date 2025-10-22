// expect: true-true-true-true-true
// Test delete on non-reference values (literals, expressions)
// Per ECMAScript spec, delete returns true for non-references

let a = delete 1;
let b = delete "string";
let c = delete true;
let d = delete (1 + 2);
let e = delete typeof "foo";

a + "-" + b + "-" + c + "-" + d + "-" + e

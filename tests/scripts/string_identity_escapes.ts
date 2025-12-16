// expect: true
// Test JavaScript identity escape sequences
// Unknown escape sequences like \A should just yield the character itself

const test1 = "\A" === "A";
const test2 = "\Z" === "Z";
const test3 = "\z" === "z";
const test4 = "\q" === "q";
const test5 = "\E" === "E";

test1 && test2 && test3 && test4 && test5;

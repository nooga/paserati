// Test string comparison operators (TypeScript allows these)
// expect: abc<bcd abc<=bcd bcd>abc bcd>=abc

const a = "abc";
const b = "bcd";

const lt = a < b;      // true
const lte = a <= b;    // true
const gt = b > a;      // true
const gte = b >= a;    // true

(lt ? "abc<bcd" : "fail") + " " +
(lte ? "abc<=bcd" : "fail") + " " +
(gt ? "bcd>abc" : "fail") + " " +
(gte ? "bcd>=abc" : "fail");

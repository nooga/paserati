// expect: pass
// Tests regex pattern /=/ which should not be confused with /= operator
const re = /=/;
const test1 = re.test("a=b");  // Should be true
const test2 = re.test("abc");  // Should be false
(test1 === true && test2 === false) ? "pass" : "fail"

// expect: pass
// Tests that /= operator works correctly and is not confused with regex
let x = 10;
x /= 2;
const test1 = x === 5;

let y = 100;
y /= 10;
const test2 = y === 10;

(test1 && test2) ? "pass" : "fail"

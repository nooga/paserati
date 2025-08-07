// Basic with statement test
// expect: 42

let obj = { x: 42 };
let result;
with (obj) {
    result = x;
}
result;
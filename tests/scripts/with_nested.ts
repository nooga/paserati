// Test nested with statements
// expect: 123

let obj1 = { a: 10 };
let obj2 = { b: 20 };
let result;
with (obj1) {
    with (obj2) {
        result = a + b + 93; // 10 + 20 + 93 = 123
    }
}
result;
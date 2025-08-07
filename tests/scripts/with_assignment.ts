// Test with statement property assignment
// expect: 100

let obj = { x: 42 };
with (obj) {
    x = 100;
}
obj.x;
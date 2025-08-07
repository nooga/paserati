// Test for VM debugging
let obj = { x: 42 };
with (obj) {
    console.log(x);
}
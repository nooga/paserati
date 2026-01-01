// expect: 42
// no-typecheck
// Test direct eval reading caller's local variable

function test() {
    let x = 42;
    return eval("x");
}
test();

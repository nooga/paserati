// expect: 100
// no-typecheck
// Test direct eval writing to caller's local variable

function test() {
    let x = 42;
    eval("x = 100");
    return x;
}
test();

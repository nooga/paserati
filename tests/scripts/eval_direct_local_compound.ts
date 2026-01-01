// expect: 52
// no-typecheck
// Test direct eval with compound assignment to caller's local variable

function test() {
    let x = 42;
    eval("x += 10");
    return x;
}
test();

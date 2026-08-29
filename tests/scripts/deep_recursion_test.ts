// Test for stack overflow with deep recursion
// no-typecheck
// expect_runtime_error: Stack overflow

function recurse(n) {
    if (n <= 0) return 0;
    return 1 + recurse(n - 1);
}
// 20000 exceeds the VM's maximum call-stack depth (MaxFrames)
console.log(recurse(20000));

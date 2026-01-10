// Test for stack overflow with deep recursion
// no-typecheck
// expect_runtime_error: Stack overflow

function recurse(n) {
    if (n <= 0) return 0;
    return 1 + recurse(n - 1);
}
// 2000 exceeds the 1024 frame limit
console.log(recurse(2000));

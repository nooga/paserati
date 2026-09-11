// Test for stack overflow with deep recursion
// no-typecheck
// #407: ordinary recursive calls now throw a catchable RangeError with the
// same spec-correct message the `new`-expression overflow path already
// used, instead of an uncatchable-as-RangeError generic "Stack overflow".
// expect_runtime_error: Maximum call stack size exceeded

function recurse(n) {
    if (n <= 0) return 0;
    return 1 + recurse(n - 1);
}
// 20000 exceeds the VM's maximum call-stack depth (MaxFrames)
console.log(recurse(20000));

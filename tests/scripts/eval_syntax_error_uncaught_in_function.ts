// An uncaught SyntaxError from direct eval inside a function must propagate
// as an ordinary uncaught exception, not crash the VM (#559).
// no-typecheck
// expect_runtime_error: SyntaxError
function m() { return eval("a b"); }
m();

// expect: 100
// no-typecheck
// Test that initialized vars work correctly in closures (this should pass)

function Test() {
    function inner() { return x; }
    var x = 100;  // Initialized in declaration
    return inner();
}

Test();

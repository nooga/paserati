// expect: 7
// no-typecheck
// Test that Function.prototype.apply correctly handles the arguments object
function wrapper() {
    function target(a, b) {
        return a + b;
    }
    return target.apply(null, arguments);
}
wrapper(3, 4);

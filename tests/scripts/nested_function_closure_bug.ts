// expect: function,inner
// no-typecheck
// Bug: function declarations don't capture outer var declarations correctly
// Test that nested function declarations can access outer var declarations
function Outer() {
    var inner = function() { return "inner"; };

    function useInner() {
        return typeof inner + "," + inner();
    }

    this.run = function() {
        return useInner();
    }
}

var o = new Outer();
o.run();

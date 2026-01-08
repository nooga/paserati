// expect: foo
// Test that nested constructor calls preserve correct prototype
function A() { this.x = 1; }
A.prototype.foo = function() { return "foo"; }

function B() { this.a = new A(); }

var b = new B();
b.a.foo();

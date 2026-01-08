// expect: set string
// no-typecheck
// Test that static accessor setters work correctly on classes
var stringSet;
class C {
    static get foo() { return 'get string'; }
    static set foo(v) { stringSet = v; }
}
C.foo = 'set string';
stringSet;

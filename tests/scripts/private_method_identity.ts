// expect: true
// Test that private methods return the same function object on repeated access
class C {
    #foo() { return 42; }
    getMethod() { return this.#foo; }
}
const c = new C();
c.getMethod() === c.getMethod();

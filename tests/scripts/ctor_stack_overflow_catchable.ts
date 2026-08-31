// expect: true

// #135: constructor recursion that overflows the call stack must raise a
// catchable RangeError ("Maximum call stack size exceeded"), matching
// V8/Node semantics - not an internal, uncatchable engine error.
function f(): boolean {
    class Foo {
        constructor(n: number) {
            new Foo(n + 1);
        }
    }
    try {
        new Foo(0);
        return false;
    } catch (e) {
        return e instanceof RangeError;
    }
}

f();

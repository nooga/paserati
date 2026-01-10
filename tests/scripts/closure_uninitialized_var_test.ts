// expect: 100
// no-typecheck
// BUG: Tests closure capture of uninitialized vars - currently broken

function Test() {
    function inner() { return x; }
    var x;
    x = 100;
    return inner();
}

Test();

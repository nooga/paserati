// expect: 100
// no-typecheck
// Tests closure capture of vars declared after inner function

function Test() {
    function inner() { return x; }
    var x;
    x = 100;
    return inner();
}

Test();

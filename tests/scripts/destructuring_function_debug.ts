// Debug function parameter destructuring
function test([a, b]) {
    return "a=" + a + " b=" + b;
}

let result = test(["X", "Y"]);
result;
// expect: a=X b=Y
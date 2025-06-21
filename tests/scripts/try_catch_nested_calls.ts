// Catching exceptions from nested function calls
function inner() {
    throw "deep error";
}

function middle() {
    inner();
}

function outer() {
    try {
        middle();
    } catch (e) {
        return e;
    }
}

outer();
// expect: deep error
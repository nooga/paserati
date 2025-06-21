// Nested try-catch blocks
let result;
try {
    try {
        throw "inner error";
    } catch (inner) {
        throw "outer error";
    }
} catch (outer) {
    result = outer;
}
result;
// expect: outer error
// Basic try-catch at top level
let result = "not caught";
try {
    throw "test error";
} catch (e) {
    result = e;
}
result;
// expect: test error
// Try-catch with no exception thrown
let result = "initial";
try {
    result = "try executed";
} catch (e) {
    result = "catch executed";
}
result;
// expect: try executed
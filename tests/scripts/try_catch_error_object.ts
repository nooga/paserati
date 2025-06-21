// Try-catch with Error object
let result;
try {
    throw new Error("Something went wrong");
} catch (e) {
    result = e.message;
}
result;
// expect: Something went wrong
// Test catch clause with object destructuring
// expect: 404

let code = 0;
try {
    throw {message: "test error", code: 404};
} catch ({message, code: c}) {
    code = c;
}
code;

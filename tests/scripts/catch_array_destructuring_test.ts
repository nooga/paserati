// Test catch clause with array destructuring
// expect: 3

let result = 0;
try {
    throw [1, 2];
} catch ([x, y]) {
    result = x + y;
}
result;

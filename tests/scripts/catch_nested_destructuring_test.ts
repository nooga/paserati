// Test catch clause with nested destructuring
// expect: 6

let sum = 0;
try {
    throw [[1, 2], 3];
} catch ([[a, b], c]) {
    sum = a + b + c;
}
sum;

// expect: 15
// Test creating TypedArray from regular array

const regular = [1, 2, 3, 4, 5];
const typed = new Uint8Array(regular);

// Sum all elements
let sum = 0;
for (let i = 0; i < typed.length; i++) {
    sum += typed[i];
}
sum;
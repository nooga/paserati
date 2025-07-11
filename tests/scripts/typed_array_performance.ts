// expect: 499500
// Performance test: Sum of array elements

const size = 1000;
const arr = new Float64Array(size);

// Initialize with values
for (let i = 0; i < size; i++) {
    arr[i] = i;
}

// Sum all elements
let sum = 0;
for (let i = 0; i < size; i++) {
    sum += arr[i];
}

sum;
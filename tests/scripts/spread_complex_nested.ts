// Test complex nested spread operations
let matrix = [[1, 2], [3, 4]];
let extra = [5, 6];
let flatten = [...matrix[0], ...matrix[1], ...extra];
flatten;
// expect: [1, 2, 3, 4, 5, 6]
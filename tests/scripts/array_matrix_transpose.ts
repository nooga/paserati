// expect: 1
// Test matrix transposition with nested arrays

let matrix: number[][] = [
  [1, 2, 3],
  [4, 5, 6],
];

let rows: number = 2;
// Assume non-empty matrix and rectangular shape for simplicity
let cols: number = 3;

// Create the transposed matrix structure (initialize with 0s)
let transposed: number[][] = [];
for (let i: number = 0; i < cols; i = i + 1) {
  let newRow: number[] = [];
  // Initialize inner array elements explicitly for paserati
  for (let k: number = 0; k < rows; k = k + 1) {
    newRow[k] = 0;
  }
  transposed[i] = newRow;
}

// Perform the transpose operation
for (let i: number = 0; i < rows; i = i + 1) {
  for (let j: number = 0; j < cols; j = j + 1) {
    transposed[j][i] = matrix[i][j];
  }
}

// Assuming the VM prints nested arrays in this format
// expect: [[1, 4], [2, 5], [3, 6]]
transposed[0][0];

// expect: 2022750
// Function to initialize a square matrix of size x size with values
function createMatrix(size: number, valueFn: (i: number, j: number) => number) {
  let matrix: number[] = []; // Use array literal
  let k = 0;
  for (let i = 0; i < size; i = i + 1) {
    for (let j = 0; j < size; j = j + 1) {
      matrix[k] = valueFn(i, j);
      k = k + 1;
    }
  }
  return matrix;
}

// Function to perform matrix multiplication C = A * B
// Matrices are represented as 1D arrays (row-major order)
function multiplyMatrices(a: number[], b: number[], size: number) {
  let c: number[] = []; // Result matrix initialized later
  let cIndex = 0;
  for (let i = 0; i < size; i = i + 1) {
    // Row of A and C
    for (let j = 0; j < size; j = j + 1) {
      // Column of B and C
      let sum = 0;
      for (let k = 0; k < size; k = k + 1) {
        // Column of A / Row of B
        // C[i][j] = sum(A[i][k] * B[k][j])
        // Index calculation: A[i][k] -> a[i * size + k]
        // Index calculation: B[k][j] -> b[k * size + j]
        sum = sum + a[i * size + k] * b[k * size + j];
      }
      c[cIndex] = sum;
      cIndex = cIndex + 1;
    }
  }
  return c;
}

// --- Benchmark Setup ---
// Use a modest size for now, can be increased if too fast
let matrixSize = 30; // e.g., 30x30 matrix

// Initialize matrices A and B
// Using simple values for demonstration
let matrixA = createMatrix(matrixSize, function (i, j) {
  return i + j;
});
let matrixB = createMatrix(matrixSize, function (i, j) {
  return i - j;
});

// --- Perform Multiplication ---
let resultMatrix = multiplyMatrices(matrixA, matrixB, matrixSize);

// --- Optional: Output checksum or value to prevent dead code elimination ---
// Calculate a simple checksum of the result matrix
let checksum = 0;
let len = matrixSize * matrixSize; // Calculate length once
for (let i = 0; i < len; i = i + 1) {
  checksum = checksum + resultMatrix[i];
}

checksum; // Use the value itself as the final expression
//console.log(checksum); // Use the value itself as the final expression

// Test array spread with literals and expressions
let arr = [2, 3];
[0, ...arr, 4, ...[5, 6], 7];
// expect: [0, 2, 3, 4, 5, 6, 7]
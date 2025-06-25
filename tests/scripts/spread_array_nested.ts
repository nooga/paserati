// Test nested array spread with function calls
let getArray = () => [10, 20];
let nested = [[1, 2], [3, 4]];
[...nested[0], ...getArray(), ...nested[1]];
// expect: [1, 2, 10, 20, 3, 4]
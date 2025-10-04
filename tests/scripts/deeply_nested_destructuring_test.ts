// expect: 42
// Test deeply nested destructuring
let [[[x]]] = [[[42]]];
x;

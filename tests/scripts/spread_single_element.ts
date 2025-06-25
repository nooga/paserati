// Test spread with single element arrays
let single = [42];
[100, ...single, 200];
// expect: [100, 42, 200]
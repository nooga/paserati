// Test spread with empty arrays
let empty = [];
let nums = [1, 2, 3];
[...empty, ...nums, ...empty];
// expect: [1, 2, 3]
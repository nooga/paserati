// Test spread syntax edge cases
let nums = [1, 2, 3];
let letters = ["a", "b"];

// Multiple consecutive spreads
let consecutive = [...nums, ...letters, ...nums];
consecutive;
// expect: [1, 2, 3, "a", "b", 1, 2, 3]
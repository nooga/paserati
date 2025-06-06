let x = null;
let y = undefined;
let z = 42;

// This would previously use many registers for null/undefined checks
let result1 = x === null;
let result2 = y === undefined;
let result3 = z === null || z === undefined;

// Test chained assignments in declarations
// This is valid JavaScript: the RHS of a const/let/var can contain assignment expressions

// Simple chained assignment
let b;
const a = b = 1;
console.log("simple:", a, b);

// Chained assignment with member expressions
const obj1 = { val: 0 };
const obj2 = { val: 0 };
const result = obj1.val = obj2.val = 42;
console.log("member:", result, obj1.val, obj2.val);

// Chained assignment with computed properties
const arr = [0, 0];
const idx = arr[0] = arr[1] = 5;
console.log("computed:", idx, arr[0], arr[1]);

// Triple chain
let x, y, z;
const w = x = y = z = 100;
console.log("triple:", w, x, y, z);

("chained_assignment_passed");

// expect: chained_assignment_passed

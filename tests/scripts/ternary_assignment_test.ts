// Test ternary expressions with assignments in branches
// In JavaScript, assignment has lower precedence than ternary condition
// but higher than the ternary operator itself, so:
// a ? b = 1 : c = 2 is parsed as a ? (b = 1) : (c = 2)

let a, b, c;

// Simple ternary with assignment in consequent
a = true;
b = 0;
const r1 = a ? b = 1 : 2;
console.log("consequent assign:", r1, b);

// Simple ternary with assignment in alternate
a = false;
c = 0;
const r2 = a ? 1 : c = 2;
console.log("alternate assign:", r2, c);

// Ternary with assignments in both branches
a = true;
b = 0;
c = 0;
const r3 = a ? b = 10 : c = 20;
console.log("both assign true:", r3, b, c);

a = false;
b = 0;
c = 0;
const r4 = a ? b = 10 : c = 20;
console.log("both assign false:", r4, b, c);

// Nested ternary with assignment
a = true;
b = false;
c = 0;
const r5 = a ? (b ? c = 1 : c = 2) : c = 3;
console.log("nested:", r5, c);

// Assignment to member expression in ternary
const obj = { val: 0 };
a = true;
const r6 = a ? obj.val = 5 : 0;
console.log("member assign:", r6, obj.val);

// Assignment to computed property in ternary
const arr = [0];
a = false;
const r7 = a ? 0 : arr[0] = 7;
console.log("computed assign:", r7, arr[0]);

("ternary_assignment_passed");

// expect: ternary_assignment_passed

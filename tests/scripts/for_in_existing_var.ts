// For...in loop with existing variable (currently has same issue as for...of)
let obj = { x: 10, y: 20 };
let key: string;

for (key in obj) {
    console.log(key);
}

// expect: y
// Now working correctly with existing variable assignment
key
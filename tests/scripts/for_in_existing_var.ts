// For...in loop with existing variable (currently has same issue as for...of)
let obj = { x: 10, y: 20 };
let key: string;

for (key in obj) {
    console.log(key);
}

// Current behavior - existing variable assignment not working correctly
// expect: undefined
// expect: null
// expect: undefined
// expect: null
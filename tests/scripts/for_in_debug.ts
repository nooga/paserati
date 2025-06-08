// Debug for...in with existing variable
let obj = { a: 1 };
let key: string;

console.log("Before loop, key:", key);

for (key in obj) {
    console.log("In loop, key:", key);
}

console.log("After loop, key:", key);

// Current behavior - existing variable assignment not working correctly 
// expect: Before loop, key: undefined
// expect: null
// expect: In loop, key: undefined
// expect: null
// expect: After loop, key: undefined
// expect: null
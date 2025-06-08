// Debug for...in with existing variable
let obj = { a: 1 };
let key: string;

console.log("Before loop, key:", key);

for (key in obj) {
    console.log("In loop, key:", key);
}

console.log("After loop, key:", key);

// expect: a
// Now working correctly with existing variable assignment
key
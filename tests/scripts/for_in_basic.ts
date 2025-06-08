// expect: ok

// Basic for...in loop test
let obj = { a: 1, b: 2, c: 3 };

console.log("Object keys:");
for (let key in obj) {
    console.log(key);
}

("ok");
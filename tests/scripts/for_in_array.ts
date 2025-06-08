// expect: ok

// For...in loop with array (should enumerate indices)
let arr = ["x", "y", "z"];

console.log("Array indices:");
for (let index in arr) {
    console.log(index);
}

("ok");
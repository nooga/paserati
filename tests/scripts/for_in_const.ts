// expect: ok

// For...in loop with const variable
let obj = { name: "Alice", age: 30 };

console.log("Object properties:");
for (const prop in obj) {
    console.log(prop);
}

("ok");
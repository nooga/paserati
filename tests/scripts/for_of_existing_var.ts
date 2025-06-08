// Test for...of with existing variable
let arr = ["x", "y"];
let item: string;

console.log("Before loop, item:", item);

for (item of arr) {
    console.log("In loop, item:", item);
}

console.log("After loop, item:", item);

// expect: Before loop, item: undefined
// expect: In loop, item: x
// expect: In loop, item: y
// expect: After loop, item: y
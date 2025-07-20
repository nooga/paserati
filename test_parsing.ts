// Test if parsing works now
let obj = { prop: "value" };
let arr = [1, 2, 3];
let func = () => "hello";

// Test optional computed access
console.log(obj?.["prop"]);

// Test optional call
console.log(func?.());

// Test optional array access  
console.log(arr?.[0]);

"parsing test";
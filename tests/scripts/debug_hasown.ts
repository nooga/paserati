// expect: true

// Simple test for hasOwnProperty
let obj: any = {};
obj.c = 3;
console.log("obj =", obj);
console.log("obj.hasOwnProperty('c') =", obj.hasOwnProperty("c"));

// Final statement
obj.hasOwnProperty("c");

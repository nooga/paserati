// Test Object static methods minimal
// expect: success

let obj = { a: 1 };
Object.values(obj);
Object.entries(obj);
Object.hasOwn(obj, "a");

"success";
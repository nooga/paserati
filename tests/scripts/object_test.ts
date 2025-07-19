// Test Object static methods working
// expect: done

// Test Object.values()
let obj1 = { a: 1, b: 2, c: 3 };
let values = Object.values(obj1);

// Test Object.entries()
let entries = Object.entries(obj1);

// Test Object.assign()
let target = { a: 1, b: 2 };
let source1 = { b: 3, c: 4 };
Object.assign(target, source1);

// Test Object.hasOwn()
let obj2 = { prop: "value" };
Object.hasOwn(obj2, "prop");

// Test Object.fromEntries()
let kvPairs = [["x", 10], ["y", 20]];
Object.fromEntries(kvPairs);

// Last expression value is what gets checked
"done";
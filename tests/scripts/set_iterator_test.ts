// Test Set iterators
const set1 = new Set();
set1.add("item1");
set1.add("item2");

console.log("Testing Set.values():");
const values = set1.values();
let value = values.next();
console.log("First value:", value.value);
value = values.next();
console.log("Second value:", value.value);
value = values.next();
console.log("Done:", value.done);

console.log("Testing Set.keys() (alias for values):");
const keys = set1.keys();
let key = keys.next();
console.log("First key:", key.value);
key = keys.next();
console.log("Second key:", key.value);
key = keys.next();
console.log("Done:", key.done);

console.log("Testing Set.entries():");
const entries = set1.entries();
let entry = entries.next();
console.log("First entry:", entry.value);
entry = entries.next();
console.log("Second entry:", entry.value);
entry = entries.next();
console.log("Done:", entry.done);

// expect: true
true;

// Test Map iterators
const map1 = new Map();
map1.set("key1", "value1");
map1.set("key2", "value2");

console.log("Testing Map.entries():");
const entries = map1.entries();
let entry = entries.next();
console.log("First entry:", entry.value);
entry = entries.next();
console.log("Second entry:", entry.value);
entry = entries.next();
console.log("Done:", entry.done);

console.log("Testing Map.keys():");
const keys = map1.keys();
let key = keys.next();
console.log("First key:", key.value);
key = keys.next();
console.log("Second key:", key.value);
key = keys.next();
console.log("Done:", key.done);

console.log("Testing Map.values():");
const values = map1.values();
let value = values.next();
console.log("First value:", value.value);
value = values.next();
console.log("Second value:", value.value);
value = values.next();
console.log("Done:", value.done);

// expect: true
true;

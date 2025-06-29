// Test Map constructor (basic version - no iterable support yet)
// expect: success

// Test basic constructor
let map = new Map();

// Manually add entries
map.set("key1", "value1");
map.set("key2", 42);
map.set(123, "number key");

// Verify entries were added correctly
let val1 = map.get("key1");
let val2 = map.get("key2");
let val3 = map.get(123);
let size = map.size;

// Verify results
(val1 === "value1" && val2 === 42 && val3 === "number key" && size === 3) ? "success" : "failed"
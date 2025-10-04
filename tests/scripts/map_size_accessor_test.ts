const map1 = new Map();
console.log("Empty map size:", map1.size);

map1.set("key1", "value1");
map1.set("key2", "value2");
console.log("Map with 2 items size:", map1.size);

map1.set("key3", "value3");
console.log("Map with 3 items size:", map1.size);

map1.delete("key1");
console.log("Map after delete size:", map1.size);

map1.clear();
console.log("Map after clear size:", map1.size);

// expect: 0
0;

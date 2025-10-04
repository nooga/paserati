const set1 = new Set();
console.log("Empty set size:", set1.size);

set1.add("item1");
set1.add("item2");
console.log("Set with 2 items size:", set1.size);

set1.add("item3");
console.log("Set with 3 items size:", set1.size);

set1.delete("item1");
console.log("Set after delete size:", set1.size);

set1.clear();
console.log("Set after clear size:", set1.size);

// expect: 0
0;

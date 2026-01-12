// Test WeakMap and WeakSet functionality
// WeakMap and WeakSet hold weak references to object keys/values

// WeakMap tests
const wm = new WeakMap();
const obj1 = { id: 1 };
const obj2 = { id: 2 };

// Test set and get
wm.set(obj1, "first");
wm.set(obj2, "second");
console.log("wm.get(obj1):", wm.get(obj1));
console.log("wm.get(obj2):", wm.get(obj2));

// Test has
console.log("wm.has(obj1):", wm.has(obj1));
console.log("wm.has({}):", wm.has({}));

// Test delete
wm.delete(obj1);
console.log("after delete wm.has(obj1):", wm.has(obj1));
console.log("after delete wm.get(obj1):", wm.get(obj1));

// Test method chaining
const wm2 = new WeakMap();
const key = {};
wm2.set(key, "value").set({ a: 1 }, "another");
console.log("chaining works:", wm2.get(key));

// WeakSet tests
const ws = new WeakSet();
const setObj1 = { name: "a" };
const setObj2 = { name: "b" };

// Test add and has
ws.add(setObj1);
ws.add(setObj2);
console.log("ws.has(setObj1):", ws.has(setObj1));
console.log("ws.has(setObj2):", ws.has(setObj2));
console.log("ws.has({}):", ws.has({}));

// Test delete
ws.delete(setObj1);
console.log("after delete ws.has(setObj1):", ws.has(setObj1));
console.log("after delete ws.has(setObj2):", ws.has(setObj2));

// Test method chaining
const ws2 = new WeakSet();
const setKey = {};
ws2.add(setKey).add({ x: 1 });
console.log("ws chaining works:", ws2.has(setKey));

// Test that non-object keys are rejected (return undefined for get, false for has)
console.log("wm.get(null):", wm.get(null as any));
console.log("wm.has(null):", wm.has(null as any));
console.log("ws.has(null):", ws.has(null as any));

("weakmap_weakset_passed");

// expect: weakmap_weakset_passed

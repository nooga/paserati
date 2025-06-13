// Test Object.prototype methods
let obj = { name: "test", value: 42 };

// Test hasOwnProperty
console.log("obj.hasOwnProperty('name'):", obj.hasOwnProperty("name"));
console.log("obj.hasOwnProperty('missing'):", obj.hasOwnProperty("missing"));

// Test the 'in' operator with prototype chain
console.log("'name' in obj:", "name" in obj);
console.log("'missing' in obj:", "missing" in obj);
console.log("'hasOwnProperty' in obj:", "hasOwnProperty" in obj); // Should be true (inherited)

// Test isPrototypeOf
let proto = Object.prototype;
console.log("Object.prototype.isPrototypeOf(obj):", proto.isPrototypeOf(obj));
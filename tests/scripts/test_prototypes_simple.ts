// Test the 'in' operator which should work
let obj = { name: "test", value: 42 };

console.log("'name' in obj:", "name" in obj);
console.log("'missing' in obj:", "missing" in obj);

// Test basic property access
console.log("obj.name:", obj.name);
console.log("obj.value:", obj.value);
// Test JSON.stringify return value
let obj = { name: "Alice" };
let jsonStr = JSON.stringify(obj);

// Log the type to verify it's a string
console.log(typeof jsonStr);
console.log(jsonStr);

// expect: string
typeof jsonStr;
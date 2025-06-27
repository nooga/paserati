// Debug object type in readonly
type SimpleObj = { name: string };
let obj = { name: "Alice" };
let readonlyObj: Readonly<SimpleObj> = obj;

console.log("Object assignment works");
"Object assignment works"; // Final expression

// expect: Object assignment works
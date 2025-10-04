var s = Symbol("test");
console.log("Symbol:", s);
console.log("typeof s:", typeof s);
console.log("s.toString():", s.toString());
console.log("s.description:", s.description);

var obj = {};
obj[s] = "symbol property";
console.log("obj[s]:", obj[s]);
console.log(
  "Object.getOwnPropertySymbols(obj):",
  Object.getOwnPropertySymbols(obj)
);

try {
  console.log("s[Symbol.iterator]:", s[Symbol.iterator]);
} catch (e) {
  console.log("Error accessing s[Symbol.iterator]:", e);
}

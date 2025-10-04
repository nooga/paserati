console.log("typeof Symbol:", typeof Symbol);
console.log("Symbol:", Symbol);
console.log("typeof Symbol === 'function':", typeof Symbol === "function");

if (typeof Symbol === "function") {
  var s = Symbol();
  console.log("Created symbol:", s);
  console.log("typeof s:", typeof s);
  console.log("s.toString():", s.toString());
} else {
  console.log("Symbol is not a function");
}

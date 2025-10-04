console.log("typeof undefined:", typeof undefined);
console.log("typeof Symbol:", typeof Symbol);

if (typeof Symbol === "undefined") {
  console.log("Symbol is undefined!");
} else {
  console.log("Symbol is defined as:", Symbol);
  console.log("typeof Symbol:", typeof Symbol);
  console.log("typeof Symbol === 'function':", typeof Symbol === "function");
}

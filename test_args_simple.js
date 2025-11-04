function* ref(a, b) {
  console.log("In generator body");
  console.log("a =", a);
  console.log("b =", b);
  console.log("typeof arguments =", typeof arguments);
  console.log("arguments =", arguments);
  if (arguments !== null && arguments !== undefined) {
    console.log("arguments.length =", arguments.length);
  }
}

console.log("Creating generator...");
const gen = ref(42, 'TC39');
console.log("Created generator, calling next()...");
gen.next();
console.log("Done");

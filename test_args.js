function* ref() {
  console.log("arguments.length =", arguments.length);
  console.log("arguments[0] =", arguments[0]);
  console.log("arguments[1] =", arguments[1]);
}

ref(42, 'TC39').next();
console.log("Done");

function* test(x: any) {
  console.log("Inside generator, x =", x);
  yield x;
}

console.log("Creating generator with argument 42");
let gen = test(42);
console.log("Calling next()");
let result = gen.next();
console.log("Result:", result);
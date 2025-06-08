function sum(a: number, b: number, c: number): number {
  console.log("sum called with a =", a, ", b =", b, ", c =", c);
  return a + b + c;
}

console.log("About to call sum with spread...");
let result = sum(...[1, 2, 3]);
console.log("Result from spread call:", result);

console.log("About to call sum normally...");
let normalResult = sum(1, 2, 3);
console.log("Result from normal call:", normalResult);
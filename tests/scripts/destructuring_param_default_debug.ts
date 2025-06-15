// Debug parameter defaults
function test([a = "DEFAULT"]) {
  return a;
}

let r1 = test(["PROVIDED"]);
let r2 = test([]);

`${r1} ${r2}`;
// expect: PROVIDED DEFAULT

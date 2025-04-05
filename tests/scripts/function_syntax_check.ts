// expect: 4
function apply(n: number, f: (n: number) => number) {
  return f(n);
}

let a = apply(1, (n) => n + 1);
let b = apply(1, function (n) {
  return n + 1;
});

a + b;

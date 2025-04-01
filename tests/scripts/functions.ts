// expect: 14

let f = function g(x) {
  if (x < 7) {
    return g(x + 1);
  }
  return x;
};

let a = function b(x) {
  if (x < 7) {
    return a(x + 1);
  }
  return x;
};

f(0) + a(0);

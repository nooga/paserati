// expect: 21

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

function c() {
  function d() {
    return 1;
  }
  const e = () => 6;
  return d() + e();
}

f(0) + a(0) + c();

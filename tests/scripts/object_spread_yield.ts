// expect: pass
function* gen() {
  var obj = {
    ...yield
  };
  return "pass";
}

var g = gen();
g.next();
g.next({a: 1}).value;

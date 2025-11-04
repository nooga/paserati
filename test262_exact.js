// Minimal assert implementation
const assert = {
  sameValue: (a, b, msg = "") => {
    if (a !== b) {
      throw new Error(`${msg} Expected SameValue(«${b}», «${a}») to be true`);
    }
  }
};

var first = 0;
var second = 0;
function* g() {
  first += 1;
  yield;
  second += 1;
};

var callCount = 0;
var obj = {
  *method([,]) {
    assert.sameValue(first, 1);
    assert.sameValue(second, 0);
    callCount = callCount + 1;
  }
};

obj.method(g()).next();
assert.sameValue(callCount, 1, 'generator method invoked exactly once');
console.log("TEST PASSED!");

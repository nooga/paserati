// Minimal assert implementation
const assert = {
  sameValue: (a, b, msg = "") => {
    console.log(`assert.sameValue(${a}, ${b}, "${msg}")`);
    if (a !== b) {
      throw new Error(`${msg} Expected SameValue(«${b}», «${a}») to be true`);
    }
  }
};

var first = 0;
var second = 0;
function* g() {
  console.log("g: entry, first=", first, "second=", second);
  first += 1;
  console.log("g: after first++, first=", first, "second=", second);
  yield;
  console.log("g: after yield, first=", first, "second=", second);
  second += 1;
  console.log("g: after second++, first=", first, "second=", second);
};

var callCount = 0;
var obj = {
  *method([,]) {
    console.log("method: entry, first=", first, "second=", second, "callCount=", callCount);
    assert.sameValue(first, 1);
    console.log("method: after first assert");
    assert.sameValue(second, 0);
    console.log("method: after second assert");
    callCount = callCount + 1;
    console.log("method: after callCount++, callCount=", callCount);
  }
};

console.log("Before obj.method(g()).next()");
obj.method(g()).next();
console.log("After obj.method(g()).next(), callCount=", callCount);
assert.sameValue(callCount, 1, 'generator method invoked exactly once');
console.log("TEST PASSED!");

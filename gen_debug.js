const assert = {
  sameValue: (a, b, c = "") => {
    console.log(`sameValue: ${a} === ${b} (${c})`);
    if (a !== b) {
      console.log(`Expected ${a} to be the same as ${b} (${c})`);
    }
  },
};

var first = 0;
var second = 0;
function* g() {
  console.log("g: about to increment first, first=", first, "second=", second);
  first += 1;
  console.log("g: incremented first, first=", first, "second=", second);
  yield;
  console.log("g: about to increment second, first=", first, "second=", second);
  second += 1;
  console.log("g: incremented second, first=", first, "second=", second);
}

var callCount = 0;
var obj = {
  *method([,]) {
    console.log("method: entered body, first=", first, "second=", second);
    assert.sameValue(first, 1);
    console.log("method: after first assert, callCount=", callCount);
    assert.sameValue(second, 0);
    console.log("method: after second assert, callCount=", callCount);
    callCount = callCount + 1;
    console.log("method: after increment, callCount=", callCount);
  },
};

console.log("Before obj.method(g()): first=", first, "second=", second);
obj.method(g()).next();
console.log("After obj.method(g()).next(): first=", first, "second=", second);
assert.sameValue(callCount, 1, "generator method invoked exactly once");

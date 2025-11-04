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
  first += 1;
  yield;
  second += 1;
}

var callCount = 0;
var obj = {
  *method([,]) {
    assert.sameValue(first, 1);
    assert.sameValue(second, 0);
    callCount = callCount + 1;
  },
};

obj.method(g()).next();
assert.sameValue(callCount, 1, "generator method invoked exactly once");

const assert = {
  sameValue: (a, b, c = "") => {
    console.log(`sameValue: ${a} === ${b} (${c})`);
    if (a !== b) {
      console.log(`Expected ${a} to be the same as ${b} (${c})`);
    }
  },
};

function $DONE() {
  console.log("DONE");
}

var initCount = 0;
var iterCount = 0;
var iter = (function* () {
  iterCount += 1;
})();

var callCount = 0;
var C = class {
  async *method([
    [] = (function () {
      initCount += 1;
      return iter;
    })(),
  ]) {
    assert.sameValue(initCount, 1);
    assert.sameValue(iterCount, 0);
    callCount = callCount + 1;
  }
};

new C()
  .method([])
  .next()
  .then(() => {
    assert.sameValue(callCount, 1, "invoked exactly once");
  })
  .then($DONE, $DONE);

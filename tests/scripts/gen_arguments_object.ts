// Test generator method with array destructuring parameter containing elision
// This mimics Test262: language/expressions/object/dstr/gen-meth-ary-ptrn-elision.js
// expect: 1
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
    callCount = callCount + 1;
  }
};

obj.method(g()).next();
callCount;

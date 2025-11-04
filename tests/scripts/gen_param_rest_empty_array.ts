// Test generator with rest param containing empty array pattern
// This mimics Test262 test: language/expressions/generators/dstr/ary-ptrn-rest-ary-empty.js
// expect: 1
var callCount = 0;
var f = function*([...[]]) {
  callCount = callCount + 1;
};

var iter = function*() {}();
f(iter).next();
callCount;

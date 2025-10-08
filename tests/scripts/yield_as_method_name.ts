// expect: 42
// @ts-nocheck
var obj = {
  yield() { return 42; }
};
obj.yield();

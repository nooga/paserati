// expect: 42
// @ts-nocheck
var obj = {
  get yield() { return 42; }
};
obj.yield;

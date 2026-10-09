// expect: undefined
// no-typecheck
var obj = {
  set yield(val) { },
  get yield() { return undefined; }
};
obj.yield = 100;
obj.yield;

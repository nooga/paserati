// Named function expression - assignment to name should be silently ignored
// expect: true
let ref = function f() {
  f = 1 as any;  // Should be silently ignored
  return typeof f === "function";
};
ref();

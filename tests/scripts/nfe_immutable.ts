// Named function expression - assignment to name throws TypeError in strict mode
// (TypeScript mode is always strict)
// expect_runtime_error: Assignment to constant variable 'f'
let ref = function f() {
  f = 1 as any;  // Should throw TypeError in strict mode
  return typeof f === "function";
};
ref();

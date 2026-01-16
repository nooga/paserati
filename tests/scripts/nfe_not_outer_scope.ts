// Named function expression - name should NOT be visible in outer scope
// expect: undefined
let ref = function innerName() {
  return 42;
};
typeof innerName;

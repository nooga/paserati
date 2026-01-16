// Named function expression - name should be accessible inside
// expect: function
let ref = function f() {
  return typeof f;
};
ref();

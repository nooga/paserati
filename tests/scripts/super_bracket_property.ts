// Test super[expr] property access
// expect: a
// no-typecheck

var fromA;
var A = { fromA: 'a', fromB: 'a' };
var B = { fromB: 'b' };
Object.setPrototypeOf(B, A);

var obj = {
  fromA: 'c',
  fromB: 'c',
  method() {
    fromA = super['fromA'];
  }
};

Object.setPrototypeOf(obj, B);
obj.method();
fromA;

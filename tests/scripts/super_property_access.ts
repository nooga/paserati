// Test super property access looks up prototype chain
// expect: a

var A = { fromA: 'a', fromB: 'a' };
var B = { fromB: 'b' };
Object.setPrototypeOf(B, A);

var obj = {
  fromA: 'c',
  fromB: 'c',
  method() {
    return super.fromA;  // Should get 'a' from A (not 'c' from obj)
  }
};

Object.setPrototypeOf(obj, B);
obj.method();

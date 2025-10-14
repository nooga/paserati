// Test super property access looks up prototype chain for second property
// expect: b

var A = { fromA: 'a', fromB: 'a' };
var B = { fromB: 'b' };
Object.setPrototypeOf(B, A);

var obj = {
  fromA: 'c',
  fromB: 'c',
  method() {
    return super.fromB;  // Should get 'b' from B (not 'c' from obj)
  }
};

Object.setPrototypeOf(obj, B);
obj.method();

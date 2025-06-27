// Test simple readonly assignment
class Test {
  readonly x = 10;
  y = 20;
}

let obj = new Test();
console.log(obj.x); // Should work - reading readonly
console.log(obj.y); // Should work - reading normal

obj.y = 30; // Should work - writing to normal property
console.log(obj.y);

obj.x = 40; // Should fail - writing to readonly property

// expect_compile_error: cannot assign to readonly property

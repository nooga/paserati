// Test static readonly modifier combination
class Test {
  static readonly version = "1.0";
  static readonly count = 0; // Both orders should work
  static readonly id = 42;
}

console.log(Test.version);
console.log(Test.count);
console.log(Test.id);

Test.id;
// expect: 42

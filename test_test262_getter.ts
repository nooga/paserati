// Test262-style getter test
var obj = {
  get x() { return 42; }
};

if (obj.x !== 42) {
  throw new Error('Getter failed');
}

console.log('test262 getter test passed');
// Test working getter
var obj = {
  get value() {
    return 42;
  }
};

console.log('Getter result:', obj.value);
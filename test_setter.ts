// Test setter
var obj = {
  _value: 0,
  get value() {
    return this._value;
  },
  set value(val) {
    this._value = val * 2;
  }
};

console.log('Initial:', obj.value);
obj.value = 5;
console.log('After setting 5:', obj.value);
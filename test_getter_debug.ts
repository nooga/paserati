// Debug getter access
var obj = {
  get test() {
    return 'hello';
  }
};

console.log('Object:', obj);
console.log('Accessing obj.test:', obj.test);
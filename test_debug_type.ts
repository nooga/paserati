// Debug object type to see what properties are stored
var obj = {
  get test() {
    return 'hello';
  }
};

console.log('typeof obj:', typeof obj);
console.log('obj keys:', Object.keys(obj));
// Test simple getter and computed getter
var obj = {
  get test() {
    return 'hello';
  },
  get ['computed']() {
    return 'computed value';
  }
};

console.log('Simple getter:', obj.test);
console.log('Computed getter:', obj.computed);
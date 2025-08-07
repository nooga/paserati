// Test case to reproduce object property parsing issue
let obj = {
    writable: true,
    enumerable: false,
    configurable: true
};
console.log(obj);
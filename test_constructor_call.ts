// Test constructor with .call()
function MyConstructor(value: number) {
  console.log("MyConstructor called, this:", typeof this);
  this.value = value;
  console.log("Set this.value to:", value);
}

console.log("=== Test 1: Direct new ===");
let obj1 = new MyConstructor(10);
console.log("obj1.value:", obj1.value);

console.log("\n=== Test 2: Using call ===");
let obj2: any = {};
console.log("Before call, obj2:", obj2);
MyConstructor.call(obj2, 20);
console.log("After call, obj2.value:", obj2.value);
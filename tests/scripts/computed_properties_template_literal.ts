// Test computed properties with template literals
const prefix = "test";

const obj = {
  [`${prefix}Key`]: "template literal computed property"
};

console.log(obj.testKey);

obj.testKey;

// expect: template literal computed property
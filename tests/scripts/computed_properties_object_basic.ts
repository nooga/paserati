// Test basic computed properties in object literals
const key = "dynamicKey";
const num = 42;

const obj = {
  [key]: "value1",
  ["literal" + "Key"]: "value2",
  [num]: "value3"
};

console.log(obj.dynamicKey);
console.log(obj.literalKey);
console.log(obj[42]);

obj.dynamicKey;

// expect: value1
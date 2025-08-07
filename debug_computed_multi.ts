const key = "dynamicKey";
const obj = {
  [key]: "value1",
  ["literal" + "Key"]: "value2"
};
console.log(obj.dynamicKey, obj.literalKey);
// expect: 42

let proto = { x: 42 };
let obj = Object.create(proto);
let result = obj.x;
console.log(result);
result;

// Test advanced optional chaining features: obj?.[expr] and func?.()
// expect: optional chaining advanced

// Test optional computed access on null/undefined
let nullObj = null;
let undefinedObj = undefined;

console.log(nullObj?.["prop"]);
// expect: undefined

console.log(undefinedObj?.["prop"]);
// expect: undefined

// Test optional computed access on valid objects
let obj = { name: "test", arr: [1, 2, 3] };

console.log(obj?.["name"]);
// expect: test

console.log(obj?.["arr"]?.[0]);
// expect: 1

// Test optional computed access with dynamic keys
let key = "name";
console.log(obj?.[key]);
// expect: test

// Test optional function call on null/undefined
let nullFunc = null;
let undefinedFunc = undefined;

console.log(nullFunc?.());
// expect: undefined

console.log(undefinedFunc?.());
// expect: undefined

// Test optional function call on valid functions
let func = () => "hello world";
console.log(func?.());
// expect: hello world

// Test optional function call with arguments
let funcWithArgs = (x, y) => x + y;
console.log(funcWithArgs?.(5, 3));
// expect: 8

// Test chaining optional calls and access
let complexObj = {
    getFunc: () => () => "nested call result"
};

console.log(complexObj?.getFunc?.()?.());
// expect: nested call result

// Test with null in the chain
let complexObjWithNull = {
    getFunc: () => null
};

console.log(complexObjWithNull?.getFunc?.()?.());
// expect: undefined

// Test optional access on arrays
let arr = [10, 20, 30];
console.log(arr?.[1]);
// expect: 20

let nullArr = null;
console.log(nullArr?.[0]);
// expect: undefined

// Test optional computed access with expressions
let index = 2;
console.log(arr?.[index]);
// expect: 30

console.log(nullArr?.[index + 1]);
// expect: undefined

// Success marker  
console.log("advanced optional chaining tests passed");

// Final test value for the test framework
"optional chaining advanced";
// Test String Symbol.iterator implementation
// expect: done

let str = "abc";
let iterator = str[Symbol.iterator]();

let result1 = iterator.next();
console.log("result1:", result1.value, result1.done); // "a" false

let result2 = iterator.next();
console.log("result2:", result2.value, result2.done); // "b" false

let result3 = iterator.next();
console.log("result3:", result3.value, result3.done); // "c" false

let result4 = iterator.next();
console.log("result4:", result4.value, result4.done); // undefined true

"done";
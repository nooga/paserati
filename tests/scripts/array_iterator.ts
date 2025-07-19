// Test Array Symbol.iterator implementation
// expect: done

let arr = [1, 2, 3];
let iterator = arr[Symbol.iterator]();

let result1 = iterator.next();
console.log("result1:", result1.value, result1.done); // 1 false

let result2 = iterator.next();
console.log("result2:", result2.value, result2.done); // 2 false

let result3 = iterator.next();
console.log("result3:", result3.value, result3.done); // 3 false

let result4 = iterator.next();
console.log("result4:", result4.value, result4.done); // undefined true

"done";
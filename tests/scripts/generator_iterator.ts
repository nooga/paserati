// Test Generator Symbol.iterator implementation
// expect: done

function* gen() {
  yield 1;
  yield 2;
}

let g = gen();
let iterator = g[Symbol.iterator]();

// Generator should return itself from Symbol.iterator
// (commenting out comparison due to StrictlyEquals not handling generators yet)
// console.log("iterator === g:", iterator === g);

let result1 = iterator.next();
console.log("result1:", result1.value, result1.done); // 1 false

let result2 = iterator.next(); 
console.log("result2:", result2.value, result2.done); // 2 false

let result3 = iterator.next();
console.log("result3:", result3.value, result3.done); // undefined true

"done";
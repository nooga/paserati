// Test yield* delegation with other generators
// expect: done

function* inner() {
  yield 1;
  yield 2;
}

function* outer() {
  yield* inner();
  yield 3;
}

let g = outer();
console.log(g.next().value); // 1
console.log(g.next().value); // 2
console.log(g.next().value); // 3
console.log(g.next().done);  // true

"done";
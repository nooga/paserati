// Test basic yield* delegation with arrays
// expect: done

function* gen() {
  yield* [1, 2, 3];
  yield 4;
}

let g = gen();
console.log(g.next().value); // 1
console.log(g.next().value); // 2
console.log(g.next().value); // 3
console.log(g.next().value); // 4
console.log(g.next().done);  // true

"done";
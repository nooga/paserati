// Test yield* delegation with strings
// expect: done

function* gen() {
  yield* "abc";
}

let g = gen();
console.log(g.next().value); // "a"
console.log(g.next().value); // "b"
console.log(g.next().value); // "c"
console.log(g.next().done);  // true

"done";
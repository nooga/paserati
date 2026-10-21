// expect: 3
// Assigning an IteratorResult (a lib interface) to a variable declared
// `null | IteratorResult<T> | "T"` narrows null away.
function sum(): number {
  let result: null | IteratorResult<number> | "T";
  let total = 0;
  result = step();
  if (result === "T") {
    return -1;
  }
  if (!result.done) {
    total += result.value;
  }
  return total;
}
function step(): IteratorResult<number> | "T" {
  return { done: false, value: 3 };
}
sum();

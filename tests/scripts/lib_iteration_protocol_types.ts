// expect: 6
function sum(it: Iterable<number>): number {
  let total = 0;
  for (const n of it) total += n;
  return total;
}
function* gen(): Generator<number, void, undefined> { yield 1; yield 2; }
const viaGenerator: Iterable<number> = gen();
class Counter implements Iterable<number> {
  [Symbol.iterator](): Iterator<number> {
    let i = 0;
    return { next: (): IteratorResult<number> => i < 3 ? { value: i++, done: false } : { value: undefined, done: true } };
  }
}
const r: IteratorResult<number, string> = { done: true, value: "end" };
const y: IteratorYieldResult<number> = { value: 1 };
sum([1, 2]) + sum(new Set([3])) + sum(viaGenerator) - 3 + (r.done ? 0 : 1) + y.value - 1;

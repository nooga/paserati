// A same-named `type X = typeof X` read by a hoisted function's annotation,
// and array literals against a union of tuples of different lengths (trpc's
// jsonl encoder).
// expect: 2
function encode(v: unknown): [[unknown] | [], ...[null | number, Kind, number][]] {
  if (v === undefined) return [[]];
  return [[v], [null, PROMISE, 3]];
}
const PROMISE = 0;
type PROMISE = typeof PROMISE;
const ITERABLE = 1;
type ITERABLE = typeof ITERABLE;
type Kind = PROMISE | ITERABLE;
const pair: [Kind, number] = [ITERABLE, 1];
encode(1).length;

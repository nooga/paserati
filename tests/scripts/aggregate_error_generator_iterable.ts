// expect: 2 multiple failures true
// Regression test for paserati#293: `new AggregateError(iterable, message)` used
// to panic ("value is not an object") whenever the `errors` argument was an
// iterable that wasn't a plain array or object - e.g. a generator, whose
// Symbol.iterator returns itself (a Generator value, not a plain Object).
// IterableToArray unconditionally called AsPlainObject() after only checking
// the much broader IsObject(), which also matches Generator/Set/Map/etc.
function* gen() {
  yield new Error("a");
  yield new Error("b");
}
const e = new AggregateError(gen(), "multiple failures");
`${e.errors.length} ${e.message} ${e instanceof Error}`;

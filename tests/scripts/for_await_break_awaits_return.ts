// Leaving a for-await loop early awaits the iterator's return() result
// (AsyncIteratorClose) before the code after the loop runs.
// no-typecheck
// expect: return called,return awaited,after loop
const log: string[] = [];
const it = {
  [Symbol.asyncIterator]() { return this; },
  next() { return Promise.resolve({ value: 1, done: false }); },
  return() {
    log.push("return called");
    return { then(r) { log.push("return awaited"); r({ done: true }); } };
  },
};
for await (const x of it) {
  break;
}
log.push("after loop");
log.join(",");

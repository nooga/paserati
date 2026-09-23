// expect: [][][]|[][]
// skip-typecheck
// paserati#525: iterator-helper and Temporal internal state ([[UnderlyingIterator]],
// [[Counter]], [[ISOYear]], [[EpochNanoseconds]], ...) is kept in internal slots,
// not as own properties.
const slotKeys = (v) => JSON.stringify(Reflect.ownKeys(v).map(String).filter((k) => k.startsWith("[[")));
const it = [slotKeys([1].values().map((x) => x)), slotKeys(Iterator.from({ next() { return { done: true }; } })), slotKeys([1].values().drop(0))].join("");
const tm = [slotKeys(Temporal.PlainDate.from("2020-01-01")), slotKeys(Temporal.Instant.fromEpochMilliseconds(0))].join("");
it + "|" + tm;

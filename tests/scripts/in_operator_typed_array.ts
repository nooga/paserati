// expect: true
// paserati#502: 'in' used to unconditionally throw for a TypedArray instead
// of treating it as an ordinary object, breaking real code (e.g. Express's
// `etag` dependency does `'ctime' in buffer` as a plain duck-type check).

const buf = new Uint8Array(4);

if ("foo" in buf) throw new Error("expected 'foo' not to be found");
if (!("length" in buf)) throw new Error("expected 'length' to be found");
if (!(0 in buf)) throw new Error("expected index 0 to be found");
if (10 in buf) throw new Error("expected out-of-range index not to be found");
if (!(Symbol.iterator in buf)) throw new Error("expected Symbol.iterator to be found");

true;

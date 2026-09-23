// expect: [] | ["next","return","Symbol(Symbol.toStringTag)"] | true | TypeError | TypeError | TypeError | {"done":true} | 1 | {"done":true} | TypeError | RangeError | SyntaxError | 1,2,2 | [[1,3],[2,9]] | [{"a":1,"b":3},{"a":2,"b":4}] | TypeError | TypeError
// skip-typecheck
// paserati#539: Iterator Helpers have no own properties - next/return are
// shared methods of %IteratorHelperPrototype% that brand-check their
// receiver - and run as generators: re-entering a running helper throws,
// return() closes the underlying iterator, and IteratorStep/IteratorClose
// errors propagate instead of being swallowed.
const out = [];
const it = [1, 2].values();
const h = it.map((x) => x);
const proto = Object.getPrototypeOf(h);
out.push(JSON.stringify(Reflect.ownKeys(h)));
out.push(JSON.stringify(Reflect.ownKeys(proto).map(String)));
out.push(h.next === it.drop(0).next && h.return === Iterator.concat().return);
const bad = (f) => { try { f(); return "no throw"; } catch (e) { return e.constructor.name; } };
out.push(bad(() => proto.next.call({})), bad(() => proto.return.call([].values())));
let self;
self = [1].values().map(() => bad(() => self.next()));
out.push(self.next().value);
let closed = 0;
const src = { next() { return { value: 1, done: false }; }, return() { closed++; return {}; } };
const t = Iterator.from(src).take(5);
t.next();
out.push(JSON.stringify(t.return()), closed, JSON.stringify(t.next()));
out.push(bad(() => ({ next() { return 1; }, __proto__: Iterator.prototype }).map((x) => x).next()));
out.push(bad(() => ({ next() { return { get done() { throw new RangeError(); } }; }, __proto__: Iterator.prototype }).toArray()));
const throwingReturn = { next() { return { value: 1, done: false }; }, return() { throw new SyntaxError(); }, __proto__: Iterator.prototype };
out.push(bad(() => throwingReturn.some((x) => x)));
out.push([...[1, 2, 3].values().flatMap((x) => [x, x]).drop(1).take(3)].join());
out.push(JSON.stringify([...Iterator.zip([[1, 2], [3]], { mode: "longest", padding: [0, 9] })]));
out.push(JSON.stringify([...Iterator.zipKeyed({ a: [1, 2], b: [3, 4] })]));
out.push(bad(() => Iterator.zip([[1], [2, 3]], { mode: "strict" }).toArray()));
out.push(bad(() => Iterator.zip(["ab"])));
out.join(" | ");

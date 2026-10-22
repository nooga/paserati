// expect: 3
// A rest parameter typed by a type parameter (or `[...T]`) infers the
// arguments' tuple, which a recursive conditional can then walk.
interface R<T> { r: T }
type Recs<T extends R<any>[], Acc = {}> =
  T extends [infer H extends R<any>, ...infer Tl extends R<any>[]] ? Recs<Tl, H["r"] & Acc> : Acc;
function merge<T extends R<any>[]>(...l: [...T]): Recs<T> {
  return Object.assign({}, ...l.map((x) => x.r));
}
const m = merge({ r: { a: 1 } }, { r: { b: 2 } });
m.a + m.b;

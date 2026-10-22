// expect: 15
// Tuple patterns infer element by element, a rest takes what is left, an
// object true branch is instantiated, and `infer X extends C` rejects what
// C does not admit.
type First<T> = T extends [infer H, ...infer _] ? H : never;
type Rest<T> = T extends [any, ...infer Tl] ? Tl : never;
type Wrap<T> = T extends { v: infer U } ? { w: U } : never;
type OnlyNum<T> = T extends [infer N extends number] ? N : 0;
type Merge<T extends { r: {} }[], Acc = {}> =
  T extends [infer H extends { r: {} }, ...infer Tl extends { r: {} }[]] ? Merge<Tl, H["r"] & Acc> : Acc;
let a: First<[5, "s"]> = 5;
let r: Rest<[1, 2, 3]> = [2, 3];
let w: Wrap<{ v: 4 }> = { w: 4 };
let o: OnlyNum<["x"]> = 0;
let m: Merge<[{ r: { a: 1 } }, { r: { b: 2 } }]> = { a: 1, b: 2 };
a + r[1] + w.w + o + m.a + m.b;

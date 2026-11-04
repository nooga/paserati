// expect: 6
// keyof and readonly bind tighter than & and |: `keyof O | 5` is
// (keyof O) | 5 and `readonly [..] | readonly [..]` a union of readonly
// tuples, which an array literal is contextually typed (as a tuple) by.
type O = { a: 1; b: 2 };
type K = keyof O | 5;
const k: K = 5;
type K2 = keyof O & string;
const k2: K2 = "a";
function hasKey<T extends O>(key: keyof T & string): boolean { return key === "a"; }
type SR<D> = readonly [data: D, error: undefined] | readonly [data: undefined, error: Error];
function ok<D>(d: D): SR<D> { return [d, undefined]; }
const [d] = ok(1);
(k === 5 ? 5 : 0) + (hasKey<O>(k2) ? 0 : 100) + (d ?? 0);

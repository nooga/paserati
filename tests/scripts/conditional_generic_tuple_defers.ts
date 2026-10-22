// expect: true
// `[A] extends [B]` waits for its arguments instead of being decided while A
// and B are still type parameters; an optional property read by an indexed
// access includes undefined.
type Same<A, B> = [A] extends [B] ? ([B] extends [A] ? true : false) : false;
type Wrapped<A, B> = { a: A } extends { a: B } ? true : false;
type IsAnyT<T> = [T] extends [0 & T] ? true : false;
type Check<E extends { r: boolean }> = E["r"] extends false ? "fn" : "inverted";
interface Opts { tr?: { a: 1 } }
const s: Same<1, 1> = true;
const w: Wrapped<1, 1> = true;
const o: Same<Opts["tr"], { a: 1 } | undefined> = true;
const c: Check<{ r: IsAnyT<string> }> = "fn";
s && w && o && c === "fn";

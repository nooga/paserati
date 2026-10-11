// A generic method's own type parameter, a mapped type's key and a
// conditional's `infer` shadow an outer type parameter of the same name:
// instantiating the outer one must not rewrite them.
interface Box<T> {
  value: T;
  map<T>(f: (x: number) => T): T;
}
const box: Box<string> = { value: "s", map: (f) => f(1) };
const n: number = box.map((x) => x + 1);

type Keys<key> = { held: key; each: { [key in "a" | "b"]: key } };
const each: Keys<number>["each"] = { a: "a", b: "b" };
const held: Keys<number>["held"] = 1;

type Inner<U> = { held: U; inner: { v: string } extends { v: infer U } ? U : never };
const i: Inner<number> = { held: 1, inner: "xy" };

n + held + i.held + i.inner.length + each.a.length;

// expect: 7

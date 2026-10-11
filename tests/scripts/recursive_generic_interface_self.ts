// expect: c
// `next?: Nd<T>` inside `interface Nd<T>` is the declared type itself, with
// every member, not a copy of the members declared before it.
interface Nd<T> {
  value: T;
  next?: Nd<T>;
}
const n: Nd<string> = { value: "a", next: { value: "b", next: { value: "c" } } };
n.next!.next!.value;

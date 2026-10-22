// expect: 2
// Spreading a type parameter gives an intersection with it: the literal
// `{ ...item, error }` is a T & { error: number }, so it is a T.
interface A { error: number; id: string }
interface B { result: string; id: string }
function mark<T extends A | B>(item: T): T {
  if ("error" in item) {
    return { ...item, error: 2 };
  }
  return item;
}
const marked = mark({ error: 1, id: "a" });
marked.error;

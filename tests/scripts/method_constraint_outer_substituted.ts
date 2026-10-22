// expect: 1
// A method type parameter with no inference sites solves to its constraint,
// read with the instantiated class's type arguments substituted.
class Init<C> {
  create<T extends { v: C }>(): T {
    return { v: 1 } as any;
  }
}
const made = new Init<number>().create();
const n: number = made.v;
n;

// expect: 3
type A = Awaited<Promise<Promise<number>>>;
type B = Awaited<string | Promise<boolean>>;
const a: A = 1;
const b: B = true;
const c: B = "x";
function unwrap<T>(x: T): Awaited<T> { return x as Awaited<T>; }
const d: number = 2;
const e: Awaited<number> = unwrap(5);
a + d;

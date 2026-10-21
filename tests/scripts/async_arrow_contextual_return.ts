// expect: true
// A contextually typed async arrow returns Promise<T>, also when T is inferred
const run = <TValue>(fn: () => TValue): TValue => fn();
const a: Promise<number> = run(async () => 1);
const f1: () => Promise<number> = async () => { return 2; };
const f2: () => Promise<number> = async () => 3;
const xs: Promise<number>[] = [0].map(async (x) => x);
a.catch(() => {});

a instanceof Promise && f1() instanceof Promise && f2() instanceof Promise && xs[0] instanceof Promise;

// expect: 1
function get<T, K extends keyof T>(o: T, k: K): T[K] { return o[k]; }
const n: number = get({ a: 1 }, "a");
function pair<T extends number>(x: T, y: T): T[] { return [x, y]; }
const p: (1 | 2)[] = pair(1, 2);
n;

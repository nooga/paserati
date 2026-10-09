// expect: 4
function foo2<T, U extends { length: T }>(x: T, y: U) { return y; }
foo2(1, "");
foo2(1, [] as number[]);
function len<T extends { length: number }>(x: T): number { return x.length; }
len("abcd");

// expect: 3
// A `// @ts-ignore` comment suppresses type errors on the next code line.
let n: number = 1;
// @ts-ignore
n = "2";
// @ts-expect-error
let m: number = "x";
n = 3;
n;

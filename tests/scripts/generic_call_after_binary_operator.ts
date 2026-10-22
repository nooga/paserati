// expect: 9
// Type arguments then '(' bind like a call also as an operand of a binary
// operator: `1 + f<number>(2)` is `1 + (f<number>(2))`, not a comparison.
function id<T>(x: T): T {
  return x;
}
const box = { get<T>(x: T): T { return x; } };
let a = 1;
let b = 2;
const sum = 1 + id<number>(2) + box.get<{ n: number }>({ n: 3 }).n;
const cmp = a < b && b > a;
const mixed = a < id<number>(b);
cmp && mixed ? sum + 3 : 0;

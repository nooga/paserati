// Issue #121: long operator chains compile as an iterative fold instead of by
// recursing once per term. These pin the semantics the fold must preserve —
// evaluation order, short-circuiting, and accumulator/destination aliasing.
// expect: true

const results: boolean[] = [];
let log: string[] = [];
function t(x: number): number {
  log.push(String(x));
  return x;
}

// Operands are evaluated left to right, exactly once each.
const sum = t(1) + t(2) + t(3) + t(4);
results.push(sum === 10 && log.join(",") === "1,2,3,4");

// The left operand of the innermost step is read in place only when the term
// after it cannot mutate it. Here f() does, so x must fold as 1 + 10 + 100.
let x = 1;
function f(): number {
  x = 100;
  return 10;
}
x = x + f() + x;
results.push(x === 111);

// The accumulator is both destination and left source of every step, and the
// last step writes a register that earlier terms may still be reading.
let s = "a";
s = s + s + s + s;
results.push(s === "aaaa");
let y = 2;
y = y * y * y;
results.push(y === 8);

// Precedence and associativity survive flattening.
results.push(1 + 2 * 3 - 4 + 5 === 8);
results.push(100 - 1 - 2 - 3 - 4 === 90);
results.push("" + 1 + 2 + 3 === "123");
results.push(16 >> 1 >> 1 >> 1 === 2);

// Short-circuiting stops evaluation at the deciding term.
function p<T>(v: T, n: string): T {
  log.push(n);
  return v;
}
log = [];
const andChain = p(1, "a") && p(0, "b") && p(3, "c");
results.push(andChain === 0 && log.join(",") === "a,b");

log = [];
const orChain = p(0, "a") || p(0, "b") || p(7, "c") || p(9, "d");
results.push(orChain === 7 && log.join(",") === "a,b,c");

log = [];
const nullishChain = p(null, "a") ?? p(undefined, "b") ?? p(5, "c") ?? p(6, "d");
results.push(nullishChain === 5 && log.join(",") === "a,b,c");

// ?? stops at any non-nullish value, including falsey ones.
log = [];
const falseyNullish = p(false, "a") ?? p(1, "b") ?? p(2, "c");
results.push(falseyNullish === false && log.join(",") === "a");

// Different logical operators must not fold into a shared exit: a falsey left
// short-circuits the && but the || still has to evaluate its right operand.
results.push(((0 && 1) || 42) === 42);
results.push((1 && 0 || 5) === 5);

// The null/undefined comparison peephole still applies inside a chain.
results.push((1 + 2 === null) === false);

results.every((r) => r);

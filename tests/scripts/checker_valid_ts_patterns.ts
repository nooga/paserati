// expect: 2|id_a|1|3|13|ok
// #614, #616: valid TypeScript the checker used to reject
class A { static readonly x: number = 0; }
class B extends A { static override readonly x = 1; }
interface Pair<P, Q = P> { a: P; b: Q }
const p: Pair<number> = { a: 1, b: B.x + 1 };
const tl: `id_${"a" | "b"}` = "id_a";
function spread<T extends object>(o: T) { return { ...o, x: 1 }; }
let n: number = 1;
(n as any) = 2;
(n as number)++;
n! += 10;
declare namespace Ambient { const v: string }
function dec(m: any, c: ClassMethodDecoratorContext) { return m; }
class D { @dec m() { return "ok"; } }
[p.b, tl, spread({ a: 1 }).x, B.x + 2, n, new D().m()].join("|");

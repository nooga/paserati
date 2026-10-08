// expect: 3|a,b|x|2|1,2
// Writes the readonly rules allow: mutable members, constructor init,
// mutable-to-readonly, readonly-constrained generics, as const satisfies.
interface Box { readonly id: string; count: number }
const b: Box = { id: "x", count: 1 };
b.count += 2;
class P { readonly tags: readonly string[]; constructor(t: string[]) { this.tags = t; } }
const p = new P(["a", "b"]);
const mutable: number[] = [1, 2];
const view: readonly number[] = mutable;
function len<T extends readonly unknown[]>(xs: T): number { return xs.length; }
const pair = [1, 2] as const satisfies number[];
declare function first<A, B>(v: readonly [A[], B[]]): A;
`${b.count}|${p.tags.join(",")}|${b.id}|${len(view)}|${pair.join(",")}`;

// expect: done|r|1|create
// #638: homomorphic mapped types keep `?`, T[][0], unknown | null, as const.
type RO<T> = { readonly [K in keyof T]: T[K] };
interface Def { mode?: "A" | "B"; n?: number }
const d: RO<Def> = {};
let s = "";
if (d.mode !== undefined) s += d.mode;
if (d.n !== undefined) s += d.n;
interface Defs { bindings: { role: string; members: string[] }[] }
const defs: Defs = { bindings: [{ role: "r", members: [] }] };
const existing: Defs["bindings"][0] = defs.bindings[0];
function f(): unknown | null { const r: unknown = 1; return r; }
const B = ["create", "delete"] as const;
function take(x: string): string { return x; }
function run(a: (typeof B)[number] | string | undefined): string { if (!a) return ""; return take(a); }
const C = { a: 1 } as const;
const one: 1 = C.a;
`${s || "done"}|${existing.role}|${f()}|${run("create")}`;

// expect: 3|z1|5|0|3|small big
function put(args?: { key?: string; value?: string }) {
  if (!args?.key) throw new Error("key");
  if (!args?.value) throw new Error("value");
  return args.key.length + args.value.length;
}
function find(n: string): { id?: string } | null { return n ? { id: "z1" } : null; }
function zone(n: string): string | undefined { const z = find(n); if (!z?.id) return undefined; return z.id; }
function price(sku: any): number {
  const rates = sku.info[0].expr?.rates;
  if (!rates || !Array.isArray(rates) || rates.length === 0) return 0;
  return rates[0].n;
}
type B = { role: string; members: string[] };
const bindings: B[] = [];
let b = bindings.find(x => x.role === "r");
if (!b) { b = { role: "r", members: [] }; bindings.push(b); }
function sum(xs: number[]): number | null { return xs.length ? xs.reduce((a, b) => a + b, 0) : null; }
function cost(): number | null { const total = sum([1, 2]); if (total === null || total <= 0) return null; return total; }
function f(n: number | null, m: any) { if (n !== null && n <= 3) return "small"; if (m?.v > 2) return "big"; return "x"; }
[put({ key: "ab", value: "c" }), zone("a"), price({ info: [{ expr: { rates: [{ n: 5 }] } }] }), b.members.length, cost(), f(2, { v: 3 }) + " " + f(null, { v: 3 })].join("|");

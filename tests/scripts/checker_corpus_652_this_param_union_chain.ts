// expect: cb|go|eu HIGH|1|c|2|4
function deco(_m: any, ctx: ClassMethodDecoratorContext) {
  ctx.addInitializer(function (this: any) { this.tag = String(ctx.name); });
}
function run(cb: () => void) { cb(); }
const out: string[] = [];
run(function (this: any) { out.push("cb"); });
class E { @deco go() {} }
out.push((new E() as any).tag);

interface Avail { id: string; availability: "NONE" | "LOW" | "HIGH" }
interface Gpu { id: string; memory?: number; dataCenters?: Avail[] }
interface Cpu { id: string; vcpu?: number; dataCenters?: Avail[] }
function rank<T extends { availability: string }>(list: T[]): T[] { return list.slice(); }
function gpu(): Gpu | undefined { return { id: "g", dataCenters: [{ id: "eu", availability: "HIGH" }] }; }
function cpu(): Cpu | undefined { return undefined; }
const pick = true;
const result = pick ? gpu() : cpu();
if (result) {
  const dcs = result.dataCenters;
  if (dcs && dcs.length) for (const dc of rank(dcs)) out.push(dc.id + " " + dc.availability);
}

interface Policy { alerts?: { slack?: { channel: string; url: string }[] } }
const state: { slack?: { channel: string; url: string }[] } = {};
const p: Policy = { alerts: { slack: [{ channel: "c", url: "u" }] } };
state.slack = p.alerts?.slack?.map(ch => ({ channel: ch.channel, url: ch.url }));
out.push(String(state.slack?.length));
out.push(state.slack?.[0].channel ?? "none");

function f(this: { n: number }, k: number) { return this.n + k; }
interface Y { foo<T>(this: T, arg: keyof T): string; a: number }
const y: Y = { a: 1, foo(arg) { return String(arg); } };
out.push(String(f.call({ n: 1 }, 1)), String(true ? 4 : 0 + 1));
out.join("|");

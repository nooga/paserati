// expect: 30000|b|ping|2 1|2 1|true 2 1
const out: string[] = [];

interface Opts { baseUrl?: string; headers?: Record<string, string>; timeout?: number; parseJson?: boolean }
class Client {
  private options: Required<Opts>;
  constructor(options: Opts = {}) {
    this.options = { baseUrl: options.baseUrl || "", headers: options.headers || {}, timeout: options.timeout || 30000, parseJson: options.parseJson !== false };
  }
  get t() { return this.options.timeout; }
}
out.push(String(new Client().t));

const def: { name: string; slug?: string } = { name: "a", slug: "s" };
const items: any[] = [{ name: "b", slug: "s" }];
const found = items.find((it: any) => it.name === def.name || (def.slug && it.slug === def.slug));
out.push(found?.name);

interface Conn { call<T>(m: string): T }
function connect(o: { url: string }): Conn { return { call: <T>(m: string) => m as unknown as T }; }
const lib: { connect: typeof connect } = { connect };
class B {
  private client?: ReturnType<typeof lib.connect>;
  ensure(): ReturnType<typeof lib.connect> { if (this.client) return this.client; this.client = lib.connect({ url: "x" }); return this.client; }
}
out.push(new B().ensure().call<string>("ping"));

function ids(names: string[]): string[] { return names.map(n => "sg-" + n); }
function take(a: string[] | undefined, b: string[] | undefined) { return (a?.length ?? 0) + (b?.length ?? 0); }
const names: string[] = ["a"];
const sg = names.length > 0 ? ids(names) : [];
function tags(v: string): { key: string; value: string }[] { return v ? [{ key: "owner", value: v }] : []; }
out.push(take(sg.filter(s => s !== "x"), sg) + " " + tags("me").length);

const ro: readonly string[] = ["x"];
const copy: string[] = [...ro];
out.push(copy.length + 1 + " " + copy.length);

type Kind = "a" | "b" | "c";
const NEEDS: ReadonlySet<Kind> = new Set<Kind>(["a", "b"]);
const RO: ReadonlyMap<string, number> = new Map([["k", 1]]);
out.push([NEEDS.has("a"), NEEDS.size, RO.get("k")].join(" "));
out.join("|");

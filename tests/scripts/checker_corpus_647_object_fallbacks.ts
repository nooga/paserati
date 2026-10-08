// expect: none|u{"headers":{"a":"b"}}|{"a":1}|r|true
type Meta = Record<string, string>;
class C { metadata?: Meta; f() { const meta = this.metadata || {}; return meta.version || "none"; } }

interface Opts { method: string; region?: string; headers?: Record<string, string>; body?: string; }
function get(url: string, o?: Partial<Opts>): string { return url + JSON.stringify(o); }
const headers: Record<string, string> = { a: "b" };

function spreadAny(obj: any): any { if (!obj || typeof obj !== "object") return obj; const c = { ...obj }; return c; }

function hasRole(v: unknown) { if (typeof v === "object" && v !== null && "role" in v) return (v as any).role; return "none"; }
function hasPaths(err: unknown) { return !!err && typeof err === "object" && "path" in (err as object) && "metadata" in (err as object); }

[new C().f(), get("u", { headers }), JSON.stringify(spreadAny({ a: 1 })), hasRole({ role: "r" }), hasPaths({ path: 1, metadata: 2 })].join("|");

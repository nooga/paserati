// expect: ok
// Valid programs rejected after #697 (#699).
interface Q { Delay?: number }
function rhsSameProp(q: Q): Record<string, string> {
  const out: Record<string, string> = {};
  if (q.Delay !== undefined) { out.Delay = q.Delay.toString(); }
  return out;
}

interface D { env?: { vars?: Record<string, string> }; maxAge?: number }
class C {
  definition: D = {};
  build(): Record<string, unknown> {
    const cfg: Record<string, unknown> = {};
    if (this.definition.env !== undefined) { cfg.Env = { Vars: this.definition.env.vars || {} }; }
    if (this.definition.maxAge !== undefined) { cfg.MaxAge = this.definition.maxAge.toString(); }
    return cfg;
  }
}

interface P { path?: string }
function esc(s: string): string { return s; }
function compound(d: P): string {
  let xml = "";
  if (d.path !== undefined) { xml += `<P>${esc(d.path)}</P>`; }
  return xml;
}

function req(): any { return { id: "a" }; }
let existing: any = null;
try { existing = req(); } catch { existing = null; }
if (existing) { const id: string = existing.id; }

interface Resp { statusCode: number; body: string }
interface Weak { Name?: string; Arn?: string }
function get(): Resp { return { statusCode: 200, body: "" }; }
function declaredAny(m: string): Weak {
  try {
    let response: any;
    if (m === "GET") { response = get(); } else { response = get(); }
    if (response.body) { return JSON.parse(response.body) as Weak; }
    return response;
  } catch (e) { throw e; }
}

interface R { D?: { Id?: string; Arn?: string } }
function reducedAssign(xml: string): R {
  const r: R = {};
  const m = /<D>(.*)<\/D>/.exec(xml);
  if (m) {
    r.D = {};
    const idm = /<Id>(.*?)<\/Id>/.exec(m[1]);
    if (idm) { r.D.Id = idm[1]; }
  }
  return r;
}

function v4(s: string): string | null { return s.length ? s : null; }
function nestedGuard(value: string): string | null {
  let text = value.toLowerCase();
  const tail = text.substring(1);
  if (tail.indexOf(".") !== -1) {
    const x = v4(tail);
    if (!x) { return null; }
    const o = x.split(".");
    text = text + o[0];
  }
  return text;
}

"ok";

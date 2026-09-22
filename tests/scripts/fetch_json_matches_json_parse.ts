// expect: n,__proto__,b,a,c,arr,s z,y q,p function true true true true d800 1 true true SyntaxError b,a true true 2 42 s null
// skip-typecheck
// paserati#522: Response.json() decoded through Go's map[string]any, so key
// order was random from run to run, objects had no Object.prototype, a lone
// surrogate escape became U+FFFD, and a parse error rejected with a plain
// string. Request.json() gave objects a null prototype. Both now return
// exactly what JSON.parse(await r.text()) would.
const body = '{"n":1,"__proto__":{"x":1},"b":1,"a":2,"c":{"z":1,"y":2},"arr":[{"q":1,"p":2}],"s":"\\ud800"}';
async function run() {
  const out = [];
  const o = await new Response(body).json();
  out.push(Object.keys(o).join(), Object.keys(o.c).join(), Object.keys(o.arr[0]).join());
  out.push(typeof o.hasOwnProperty, Object.getPrototypeOf(o) === Object.prototype, Object.getPrototypeOf(o.c) === Object.prototype);
  out.push(Object.hasOwn(o, "__proto__"), o.x === undefined, o.s.charCodeAt(0).toString(16), o.s.length);
  out.push(JSON.stringify(o) === JSON.stringify(JSON.parse(body)));
  try { await new Response("{bad").json(); out.push("no-throw"); } catch (e) { out.push(e instanceof SyntaxError, e.constructor.name); }
  const rq = await new Request("http://x.invalid/", { method: "POST", body: '{"b":1,"a":2}' }).json();
  out.push(Object.keys(rq).join(), Object.getPrototypeOf(rq) === Object.prototype);
  try { await new Request("http://x.invalid/", { method: "POST", body: "nope" }).json(); out.push("no-throw"); } catch (e) { out.push(e instanceof SyntaxError); }
  out.push(await new Response("[1,2]").json().then(a => Array.isArray(a) && a.length));
  out.push(await new Response("42").json(), await new Response('"s"').json(), String(await new Response("null").json()));
  return out.join(" ");
}

await run();

// expect: https://example.com/c?x=1&y=two%20words#frag|https://example.com|/c|http://ex.com/a/d?q|false|https://example.com/api?page=2&q=a+b|a=1&a=3&b=2|TypeError
// #624: URL and URLSearchParams
const u = new URL("HTTPS://Example.COM:443/a b/../c?x=1&y=two words#frag");
const rel = new URL("../d?q", "http://ex.com/a/b/c");
const api = new URL("https://example.com/api");
api.searchParams.set("page", "2");
api.searchParams.append("q", "a b");
const sp = new URLSearchParams({ b: "2", a: "1" });
sp.append("a", "3");
sp.sort();
let err = "";
try { new URL("/relative"); } catch (e: any) { err = e.name; }
[u.href, u.origin, u.pathname, rel.href, URL.canParse("nope"), String(api), sp.toString(), err].join("|");

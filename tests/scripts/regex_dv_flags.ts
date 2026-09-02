// expect: true|v|true|false|d|true|dgimsuy|dgimsvy|1,3|2,3|-|1,2|2,3|null|b|c|null|a,-|abc,a,b,c|a[b]c|a[b]c|b,b|ok|ok|ok|ok|ok
// Issue #195: the d (hasIndices) and v (unicodeSets) regex flags. The lexer
// used to reject the whole literal ("Expression expected"), and the RegExp
// constructor accepted any flag string silently. Also covers the plumbing d
// needs: exec results carry `indices` and named `groups`.
let out: string[] = [];

// v parses, is reported by flags/unicodeSets, and matches in Unicode mode.
const zeroWidth = /^(?:\p{Default_Ignorable_Code_Point}|\p{Control}|\p{Mark}|\p{Surrogate})+$/v;
out.push(String(zeroWidth.test("​‍")));
out.push(/abc/v.flags, String(/abc/v.unicodeSets), String(/abc/v.unicode));

// d parses and is reported.
out.push(/abc/d.flags, String(/abc/d.hasIndices));

// Canonical flag order, with u or with v.
out.push(/a/dgimsuy.flags, /a/dgimsvy.flags);

// indices: [start, end] in code units per capture, undefined when unmatched.
const m: any = /b(c)(x)?/d.exec("abc");
out.push(m.indices[0].join(","), m.indices[1].join(","), m.indices[2] === undefined ? "-" : "?");

// indices.groups mirrors the named captures; groups is undefined without names.
const n: any = /(?<first>b)(?<second>c)/d.exec("abc");
out.push(n.indices.groups.first.join(","), n.indices.groups.second.join(","));
out.push(String(Object.getPrototypeOf(n.indices.groups)));
out.push(n.groups.first, n.groups.second);
out.push(String(Object.getPrototypeOf(n.groups)));
const u: any = /(?<a>a).|(?<x>x)/.exec("ab");
out.push(u.groups.a + "," + (u.groups.x === undefined ? "-" : "?"));

// Mixed named/unnamed captures keep source order on the regexp2 path too
// (a lookahead forces it); regexp2 numbers named groups last.
const mixed: any = /(?<x>a)(b)(?<y>c)(?=$)/.exec("abc");
out.push(mixed.slice(0).join(","));

// $<name> in replacements, and the groups argument of a replacer function.
out.push("abc".replace(/(?<x>b)/, "[$<x>]"));
out.push("abc".replace(/(?<x>b)/, (...args: any[]) => "[" + args[args.length - 1].x + "]"));
out.push(Array.from("abab".matchAll(/(?<x>b)/g), (r: any) => r.groups.x).join(","));

// The constructor validates flags: unknown, duplicate, and u together with v.
function throwsSyntax(f: () => any): string {
  try { f(); return "no-throw"; } catch (e) { return e instanceof SyntaxError ? "ok" : "wrong"; }
}
out.push(throwsSyntax(() => new RegExp("a", "q")));
out.push(throwsSyntax(() => new RegExp("a", "gg")));
out.push(throwsSyntax(() => new RegExp("a", "uv")));
// v-mode class set operations are refused rather than silently mis-matched.
out.push(throwsSyntax(() => new RegExp("[\\p{L}--[a-z]]", "v")));
out.push(new RegExp("a", "dv").flags === "dv" ? "ok" : "bad");

out.join("|");

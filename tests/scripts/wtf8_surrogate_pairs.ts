// expect: ok
// WTF-8 canonical form: the same JS string must have one byte representation
// whether a supplementary character was written as one literal character, as
// two \uXXXX escapes, or assembled from lone surrogates by concatenation and
// friends. Byte-sensitive builtins (TextEncoder, encodeURIComponent, /u
// regexes, spread) are how the difference would show.

const results: string[] = [];
function check(name: string, cond: boolean) { if (!cond) results.push(name); }
function bytes(s: string): number { return new TextEncoder().encode(s).length; }
function units(s: string): string { const out: string[] = []; for (let i = 0; i < s.length; i++) out.push(s.charCodeAt(i).toString(16)); return out.join(" "); }

const lit = String.fromCodePoint(0x10000);
check("fromCodePoint bytes", bytes(lit) === 4 && units(lit) === "d800 dc00");
const esc = "\ud800\udc00";
check("escaped literal", esc === lit && bytes(esc) === 4);
check("escaped literal in array", bytes(["\ud800\udc00"][0]) === 4);
check("template literal", bytes(`\ud800\udc00`) === 4 && bytes(`x${"\ud800"}${"\udc00"}`) === 5);
const lead = "\ud800", trail = "\udc00";
check("lone surrogates stay", bytes(lead) === 3 && bytes(trail) === 3 && units(lead) === "d800");
check("concat", bytes(lead + trail) === 4 && lead + trail === lit);
let acc = "a" + lead; acc += trail; acc += "b";
check("compound concat", bytes(acc) === 6 && acc === "a" + lit + "b");
check("reverse order not joined", bytes(trail + lead) === 6 && units(trail + lead) === "dc00 d800");
check("String.prototype.concat", bytes(lead.concat(trail)) === 4);
check("Array.prototype.join", bytes([lead, trail].join("")) === 4 && bytes([lead, trail].join("-")) === 7);
check("padStart", bytes(trail.padStart(2, lead)) === 4 && trail.padStart(2, lead) === lit);
check("padEnd", bytes(lead.padEnd(2, trail)) === 4 && "ab".padEnd(5, lit) === "ab" + lit + lead);
check("repeat", bytes((trail + lead).repeat(2)) === 10 && units((trail + lead).repeat(2)) === "dc00 d800 dc00 d800");
check("replace", bytes(("a" + trail).replace("a", lead)) === 4 && bytes(("a" + trail).replaceAll("a", lead)) === 4);
check("regex replace", bytes(("a" + trail).replace(/a/, lead)) === 4 && bytes(("a" + trail).replace(/a/g, "$&" + lead)) === 5);
check("fromCharCode", String.fromCharCode(0xd800, 0xdc00) === lit && bytes(String.fromCharCode(0xd800, 0xdc00)) === 4 && bytes(String.fromCharCode(0xd800)) === 3);
check("fromCodePoint surrogates", String.fromCodePoint(0xd800, 0xdc00) === lit && bytes(String.fromCodePoint(0xdc00)) === 3);
check("JSON.parse", JSON.parse("\"\\ud800\\udc00\"") === lit && bytes(JSON.parse("\"\\ud800\"")) === 3 && JSON.parse("\"\\ud800\"").charCodeAt(0) === 0xd800);
check("JSON roundtrip", JSON.stringify(lead + trail) === JSON.stringify(lit) && JSON.stringify(lead) === "\"\\ud800\"");
check("spread pair", [...(lead + trail)].length === 1 && [...("a" + lit + "b")].length === 3);
check("spread lone", [...lead].length === 1 && [...lead][0] === lead && [...(trail + lead)].length === 2);
check("encodeURIComponent", encodeURIComponent(lead + trail) === "%F0%90%80%80");
check("regex /u", /^.$/u.test(lead + trail) && /^\p{Any}$/u.test(lead + trail));
check("iterator", Array.from(lead + trail).length === 1);
check("length/indexing", (lead + trail).length === 2 && (lead + trail)[0] === lead && (lead + trail).slice(1) === trail);
check("eval", bytes(eval("\"\\ud800\\udc00\"")) === 4);

results.length === 0 ? "ok" : "failed: " + results.join(", ");

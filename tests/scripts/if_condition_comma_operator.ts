// expect: 2:5:else:elseif:42
// A comma expression directly inside an if condition failed to parse with
// "')' expected" (#157). parseIfStatement parsed its condition at COMMA
// precedence - "stop before consuming a comma", correct for a comma-delimited
// context like a declarator initializer, wrong inside the condition's own
// parentheses, where a raw comma operator is unambiguous. while, do-while and
// switch all already used LOWEST; if was the sole outlier.
//
// Every left operand here has a side effect on purpose: TypeScript reports
// TS2695 ("Left side of comma operator is unused and has no side effects") for
// the pure form, so `if (x, x > 0)` is a parse test that only runs untyped.
let x = 1;
let out = "";

if ((x = 2), x > 0) out += String(x);

// The real-world idiom - assign, then test - parenthesized or not.
if ((x = 5), x > 0) out += ":" + x;

if ((x = 6), 0) out += ":BAD";
else out += ":else";

if ((x = 7), false) out += ":BAD2";
else if ((x = 8), true) out += ":elseif";

// The shape this was found on: lru-cache's minified constructor, reduced.
function isNum(v: any): boolean {
  return typeof v === "number";
}
const defaults = { perf: 42 };
class Cache {
  #perf: number = 0;
  ok: number = 0;
  constructor(p?: number, e?: any) {
    if (((this.#perf = p ?? defaults.perf), e !== 0 && !isNum(e)))
      throw new TypeError("bad e");
    this.ok = this.#perf;
  }
}
out += ":" + new Cache(undefined, 0).ok;

out;

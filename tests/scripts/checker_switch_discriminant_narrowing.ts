// expect: 4,2,3,6,b
// #615: discriminated unions narrow per switch clause (groups, default, fallthrough)
type S = { kind: "c"; r: number } | { kind: "s"; w: number } | { kind: "t"; h: number };
function g1(s: S): number { switch (s.kind) { case "c": case "s": return s.kind === "c" ? s.r : s.w; default: return s.h; } }
function g2(s: S): number { switch (s.kind) { case "c": return s.r; case "s": return s.w; default: return s.h; } }
function g3(s: S): number { switch (s.kind) { case "c": { const v = s.r; return v; } case "s": return s.w; case "t": return s.h; } }
function g4(s: S): number { let n = 0; switch (s.kind) { case "c": n = s.r; case "s": n += 1; break; case "t": n = s.h; } return n; }
function g5(v: "a" | "b" | "c"): string { switch (v) { case "a": { const t: "a" = v; return t; } default: { const u: "b" | "c" = v; return u; } } }
[g1({ kind: "t", h: 4 }), g2({ kind: "s", w: 2 }), g3({ kind: "c", r: 3 }), g4({ kind: "c", r: 5 }), g5("b")].join(",");

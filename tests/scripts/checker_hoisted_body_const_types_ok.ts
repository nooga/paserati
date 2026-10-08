// expect: create|81|local|3
// Top-level bindings read from hoisted bodies, classes and arrows get their
// inferred types; locals that shadow them are left alone.
const KINDS = ["create", "delete"] as const;
function first(): (typeof KINDS)[number] { return KINDS[0]; }
const cfg = { port: 80 };
function nextPort(): number { return cfg.port + 1; }
const name = 1;
function shadow(): string { const name = "local"; return name; }
let counter = 0;
class Counter { bump(): number { counter = counter + 1; return counter; } }
const k = new Counter();
k.bump(); k.bump();
`${first()}|${nextPort()}|${shadow()}|${k.bump()}`;

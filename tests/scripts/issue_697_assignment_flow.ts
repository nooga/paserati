// expect: ok
// Assignment narrowing survives the points where branches join (#697).
declare function find(): string | null;
declare function create(): string;
declare function use(x: string): void;
declare function useN(x: number): void;
declare const flag: boolean;

function ifElse() { let g = find(); if (g) { useN(1); } else { g = create(); } use(g); }

interface S { id?: string }
class B<T> { state!: T }
class E extends B<S> {
  run(): void { if (this.state.id) { useN(1); } else { this.state.id = create(); } use(this.state.id); }
}

function returning(id: string | undefined) { if (!id) return; if (flag) { id = undefined; return; } use(id); }
function throwing(id: string | undefined) { if (!id) return; if (flag) { id = undefined; throw new Error("x"); } use(id); }
function orAssign() { let g = find(); g = g || create(); use(g); }
function ternaryAssign() { let g = find(); g = g ? g : create(); use(g); }
function whileLoop() { let g = find(); while (!g) { g = find(); } use(g); }
function exhaustiveSwitch(k: "a" | "b") {
  let v: string | undefined;
  switch (k) { case "a": v = "x"; break; case "b": v = "y"; break; }
  use(v);
}
function finallyBlock() { let g: string | undefined; try { g = create(); } finally { useN(1); } use(g); }
function aliased(id: string | undefined) { const ok = id !== undefined; if (ok) use(id); }

"ok";

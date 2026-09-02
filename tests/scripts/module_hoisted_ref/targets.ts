// Helper for module_hoisted_ref_later_decl*.ts (#192). Every construct here
// writes to or inspects a module-level binding from inside a hoisted function
// declaration, which compiles before the declarations themselves. All of them
// used to land on a different heap slot than the declaration (bare name vs.
// module-namespaced key), so writes were lost and reads came back stale.
let a = 0;
let b = 0;
let rest: number[] = [];
let c = 0;
let d = "";
let n = 0;
class Base {
  tag = "base";
}

export function destructure(): string {
  [a] = [7];
  ({ b } = { b: 8 });
  [...rest] = [1, 2];
  return `${a}|${b}|${rest.length}`;
}

export function loops(): string {
  for (c of [3]) {
  }
  for (d in { k: 1 }) {
  }
  return `${c}|${d}`;
}

export function kind(): string {
  return typeof a;
}

export function derived(): string {
  return new (class extends Base {})().tag;
}

export function bump(): number {
  ++n;
  n++;
  n += 1;
  return n;
}

// Helper for module_hoisted_ref_later_decl*.ts (#192). Mirrors typebox's
// schema/engine/_unique.mjs: a non-exported module-level `let` that a hoisted
// function declaration mutates with ++. The function body compiles before the
// module's statements are reached, so `index` is unresolved at that point.
let index = 0;

export function Unique(): string {
  return `var_${index++}`;
}

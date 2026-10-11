// expect: 5
// `keyof`/`infer` name functions and imports; `override` names a method
// (zod declares `function keyof`, vitest a method `override`).
function keyof(x: number): number { return x + 1; }
class Fixture {
  override(n: number): number { return n * 2; }
}
keyof(new Fixture().override(2));

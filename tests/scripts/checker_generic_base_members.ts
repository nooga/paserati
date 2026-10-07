// expect: x|x2|true
// #613: readonly/protected members inherited through a generic base class
class Base1<D extends object> {
  readonly definition: D;
  constructor(d: D) { this.definition = d; }
}
class E1 extends Base1<{ name: string }> {
  f() { return this.definition.name; }
}
type DeepReadonly<T> = { readonly [K in keyof T]: T[K] extends object ? DeepReadonly<T[K]> : T[K] };
class Base2<D extends object> {
  protected readonly definition: DeepReadonly<D>;
  constructor(d: D) { this.definition = d as DeepReadonly<D>; }
}
class E2 extends Base2<{ name: string; opts: { size: number } }> {
  f() { return this.definition.name + this.definition.opts.size; }
}
class Base3<D> { protected nf() { return true; } }
class E3 extends Base3<number> { f() { return this.nf(); } }
[new E1({ name: "x" }).f(), new E2({ name: "x", opts: { size: 2 } }).f(), new E3().f()].join("|");

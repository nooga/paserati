// The valid half of declare_class_members.ts must check without errors.
declare abstract class Base<Output> {
  readonly _output: Output;
  optional(): Wrap<this>;
  parse(data: unknown): Output;
  parse(data: unknown, strict: boolean): Output | undefined;
  static create(): number;
  id<U>(u: U): U;
}
declare class Wrap<T> { inner: T; }
declare class Str extends Base<string> {}
declare namespace N { class A { n(): number; } }
function check(s: Str, a: N.A): number {
  const x: string = s._output;
  const y: Str = s.optional().inner;
  const z: string = s.parse(1);
  const w: string | undefined = s.parse(1, true);
  const v: boolean = s.id(true);
  const u: number = Base.create() + a.n();
  return 1;
}
"ok";

// expect: ok

// Method signatures of a declare class are its members: instance, static,
// overloaded and generic. A declare abstract class is parsed (it was
// skipped), and a generic class can be referenced before its declaration.
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
declare const s: Str;
const a: string = s._output;
const b: Str = s.optional().inner;
const c: string = s.parse(1);
const d: string | undefined = s.parse(1, true);
const e: number = Base.create();
const f: boolean = s.id(true);
const g: number = s.parse(1);

// expect_compile_error: Type 'string' is not assignable to type 'number'.

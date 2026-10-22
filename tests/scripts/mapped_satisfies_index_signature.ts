// expect: 2
// A generic mapped type satisfies an index-signature constraint through its
// template: a conditional's check type is known to extend its extends type
// in its own true branch, and an `infer V` there resolves once inferred.
interface Proc { p: 1 }
interface Rec { [key: string]: Proc | Rec }
interface Rtr<TRecord extends Rec> { rec: TRecord }
type Opts = { [key: string]: Proc | Rtr<any> | Opts };
type Dec<T extends Opts> = {
  [K in keyof T]: T[K] extends infer V
    ? V extends Proc ? V
    : V extends Rtr<infer R> ? R
    : V extends Opts ? Dec<V>
    : never
    : never;
};
type Built<TRecord extends Rec> = Rtr<TRecord> & TRecord;
function build<T extends Opts>(x: T): Built<Dec<T>> {
  return { rec: x, ...x } as any;
}
const p: Proc = { p: 1 };
function use(b: { foo: Proc }): number { return b.foo.p + 1; }
use(build({ foo: p }));

// A method's type parameter constrained by its class's parameters is inferred
// through a conditional parameter type (trpc's initTRPC.create), and the
// callbacks in the argument are typed by its base constraint.
// expect: 3
type ValidateShape<A, E> = A extends E
  ? Exclude<keyof A, keyof E> extends never
    ? A
    : E
  : never;
interface Shape { code: number }
type Formatter<C, S extends Shape> = (opts: { ctx: C | undefined; shape: Shape }) => S;
interface Options<C extends object> {
  isDev?: boolean;
  errorFormatter?: Formatter<C, any>;
  transformer?: { serialize: (o: any) => any };
}
type inferShape<T> = T extends Formatter<any, infer S> ? S : Shape;
class Builder<C extends object> {
  create<O extends Options<C>>(opts?: ValidateShape<O, Options<C>>): inferShape<O["errorFormatter"]> {
    return { code: 3 } as any;
  }
}
const b = new Builder<object>();
b.create();
b.create({});
b.create({ transformer: { serialize: (v) => v } });
const s: Shape = b.create({ errorFormatter: (opts) => opts.shape });
s.code;

// `new` of a generic class with nothing to infer from gives its type
// parameters their defaults, else their constraints (trpc's
// `new TRPCBuilder()` is a TRPCBuilder<object, object>).
// expect: 2
class Builder<C extends object, M extends object = { m: number }> {
  get(): [C, M] { return [{}, { m: 1 }] as any; }
}
const b = new Builder();
const pair: [object, { m: number }] = b.get();
pair.length;

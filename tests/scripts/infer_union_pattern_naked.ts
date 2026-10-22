// expect: abc
// A union pattern with a naked `infer X` takes what its other members do
// not match (tRPC's ParserCustomValidatorEsque, `Promise<X> | X`).
type Validated<F> = F extends (input: unknown) => Promise<infer X> | infer X ? X : never;
type Plain = Validated<(x: unknown) => { id: string }>;
type Async = Validated<(x: unknown) => Promise<{ id: number }>>;
const p: Plain = { id: "abc" };
const a: Async = { id: 1 };
const n: number = a.id;
p.id;

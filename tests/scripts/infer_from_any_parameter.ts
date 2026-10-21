// `any` in the checked type matches any pattern part and infers any for its
// `infer` placeholders.
// expect: ok
type Ret<T> = T extends (opts: { a: string }) => infer R ? R : "no";
type R1 = Ret<(x: any) => any>;
const r: R1 = "ok";
r;

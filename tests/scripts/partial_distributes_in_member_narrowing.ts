// expect: 1
type A = { a: 1; c: 3 };
type B = { b: 2; c: 3 };
type Req = { t: 1 } & { s?: Partial<A | B> };
const req: Req = { t: 1, s: { a: 1 } };
let r: 1 | undefined = undefined;
if (req.s && 'a' in req.s) { r = req.s.a; }
r;

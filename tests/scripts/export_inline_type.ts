// An inline `type` modifier in an export list exports only a type: no
// runtime binding is made for it.
// expect: 3
interface T { a: number }
const v = 3;
export { type T, v };
const t: T = { a: v };
t.a;

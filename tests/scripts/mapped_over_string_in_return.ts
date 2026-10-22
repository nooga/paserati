// expect: 1
// A mapped type over `keyof any` nested in a function type relates by its
// template with the key substituted (tRPC's RouterCaller<any, any>).
type M<T> = { [K in keyof T]: T[K] extends infer V ? (V extends string ? 1 : 2) : never };
type F<T> = () => M<T>;
const make = (): { foo: 1 } => ({ foo: 1 });
const f: F<any> = make;
f().foo;

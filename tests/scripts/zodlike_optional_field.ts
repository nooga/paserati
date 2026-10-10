// zod's z.object({ s: z.string().optional() }): `optional(): ZOpt<this>` names
// a class declared later, the call result is evaluated once the receiver is
// known, and addQuestionMarks + flatten keep `s` optional (`{ [K in never]: X }`
// is `{}`, `[k in keyof T]` over a concrete T keeps its modifiers).
// expect: 1
declare class ZT<O = any> { readonly _output: O; optional(): ZOpt<this>; }
declare class ZOpt<T extends ZT<any>> extends ZT<T["_output"] | undefined> {}
declare class ZStr extends ZT<string> {}
declare class ZNum extends ZT<number> {}
declare class ZObj<S, Out> extends ZT<Out> { shape: S; }
type optionalKeys<T extends object> = { [k in keyof T]: undefined extends T[k] ? k : never }[keyof T];
type requiredKeys<T extends object> = { [k in keyof T]: undefined extends T[k] ? never : k }[keyof T];
type addQuestionMarks<T extends object> = { [K in requiredKeys<T>]: T[K] } & { [K in optionalKeys<T>]?: T[K] } & { [k in keyof T]?: unknown };
type identity<T> = T;
type flatten<T> = identity<{ [k in keyof T]: T[k] }>;
type base<S> = { [k in keyof S]: S[k] extends ZT<any> ? S[k]["_output"] : never };
type output<T extends ZT<any>> = T["_output"];
function check<S extends { [k: string]: ZT<any> }>(s: S): ZObj<S, flatten<addQuestionMarks<base<S>>>> {
  return null as any;
}
const str = (): ZStr => ({ optional: () => null }) as any;
const num = (): ZNum => null as any;
const O = check({ n: num(), s: str().optional() });
type T = output<typeof O>;
const t: T = { n: 1 };
const u: T = { n: 1, s: "x" };
t.n;

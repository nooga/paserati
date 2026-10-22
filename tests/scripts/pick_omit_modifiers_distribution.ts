// expect: 6
// Pick and Omit keep the source's optional members (tsc's modifiers type),
// a distributive conditional maps each union member on its own, a union
// inside an intersection exposes its common members, and an optional
// member returning `this` relates to Partial<Class>.
interface Req {
  method?: string;
  url?: string;
  off(): void;
  kind: "req";
}
interface Res {
  method?: string;
  off(): void;
  kind: "res";
  status: number;
}
const req: Req = { kind: "req", off() {} };

const picked: Pick<Req, "method" | "kind"> = req;
const omitted: Omit<Req, "url"> = req;
const empty: Pick<Req, "method"> = {};

type DistributiveOmit<T, K extends keyof any> = T extends any ? Omit<T, K> : never;
type Message = DistributiveOmit<Req | Res, "url"> & { body?: unknown };
const msg: Message = req;
msg.off();

type Wrap<T> = T extends any ? { w: T } : never;
const wrapped: Wrap<string | number> = { w: 1 };

declare class Sock {
  pause(): this;
}
function partialSock(s: Sock): Partial<Sock> {
  return s;
}

type FilterKeys<TObj extends object, TFilter> = {
  [TKey in keyof TObj]: TObj[TKey] extends TFilter ? TKey : never;
}[keyof TObj];
type Filter<TObj extends object, TFilter> = Pick<TObj, FilterKeys<TObj, TFilter>>;
const strings: Filter<{ a: string; b: number }, string> = { a: "x" };

let count = 0;
for (const v of [picked, omitted, empty, msg, wrapped, strings]) {
  if (v) count++;
}
count;

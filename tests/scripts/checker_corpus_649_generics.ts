// expect: 1|eu HIGH|GET /|1
function parse<T>(s: string): T { return JSON.parse(s) as T; }
class Base { protected req<T>(s: string): T { return parse<T>(s); } }
class Derived extends Base { go() { return this.req<{ a: number }>('{"a":1}').a; } }

function rank<T extends { availability: string }>(list: T[]): T[] { return list.slice(); }
const dcs: { id: string; availability: string }[] = [{ id: "eu", availability: "HIGH" }];
const ranked = rank(dcs).map(dc => dc.id + " " + dc.availability);

interface Def { routes?: any }
type Route = Required<NonNullable<Def["routes"]>>[number];
const r: Route = { method: "get", path: "/" };

type DeepReadonly<T> = T extends (infer R)[] ? ReadonlyArray<DeepReadonly<R>> : T extends Function ? T : T extends object ? { readonly [P in keyof T]: DeepReadonly<T[P]> } : T;
interface W { day: "MONDAY" | "FRIDAY"; start: { hours: number } }
interface MP { description?: string; windows?: ReadonlyArray<W> }
function windowCount(p?: Readonly<MP>) { return p?.windows?.length ?? 0; }
const d: DeepReadonly<{ mp?: MP }> = { mp: { windows: [{ day: "MONDAY", start: { hours: 1 } }] } };

[new Derived().go(), ranked[0], r.method.toUpperCase() + " " + r.path, d.mp ? windowCount(d.mp) : -1].join("|");

// expect: q:a,stop
// Property reads and discriminant narrowing on unions whose members are intersections
type Id = number | string | null;
interface Req { id?: Id; method: 'query' | 'mutation'; params: { path: string } }
type Msg = (Req & { id: Id }) | ({ method: 'subscription.stop'; id?: Id } & { id: Id });

function h(msg: Msg): string {
  if (msg.id === null) { return "null"; }
  if (msg.method === 'subscription.stop') { return "stop"; }
  return "q:" + msg.params.path;
}

function idOf(u: ({ a: 1 } & { id: number }) | ({ a: 2 } & { id: number })): number {
  return u.id;
}

if (idOf({ a: 2, id: 7 }) !== 7) throw "FAIL id";
h({ id: 1, method: 'query', params: { path: "a" } }) + "," + h({ id: 2, method: 'subscription.stop' });

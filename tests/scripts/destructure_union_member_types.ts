// expect: 4
// A destructured binding keeps the property's whole union type, an
// intersection-typed union member destructures through its merged shape, and
// an array literal initializer is typed by the pattern as a tuple.
type RequestId = number | string | null;
interface Envelope {
  id?: RequestId;
  jsonrpc?: "2.0";
}
interface RpcRequest extends Envelope {
  method: "query";
}
type Message =
  | (RpcRequest & { id: RequestId })
  | (Envelope & { method: "stop"; id: RequestId });

function idOf(msg: Message): number | string {
  const { id } = msg;
  if (id === null) {
    return -1;
  }
  return id;
}

function pick(q: { v: string | boolean }): string | boolean {
  const { v } = q;
  return v;
}

let [a, [b, c]] = [1, [2, 3]];
const n: number = a + b + c;
const total = (idOf({ method: "query", id: 4 }) as number) + (pick({ v: true }) ? 0 : 1);
n - 6 + total;

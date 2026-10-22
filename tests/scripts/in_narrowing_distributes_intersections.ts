// expect: 7
// `in` narrowing sees through `{ id } & (A | B)` and generic alias
// instances: each alternative is kept or dropped on its own.
interface Ok<T> { result: T }
interface Err { error: string }
type Resp<T> = Err | Ok<T>;
type Msg<T> = { id: number } & (Err | Ok<T>);
function value(r: Resp<number> | Msg<number>): number {
  if ("error" in r) {
    return -1;
  }
  return r.result;
}
value({ id: 1, result: 7 });

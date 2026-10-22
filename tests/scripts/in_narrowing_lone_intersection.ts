// expect: 5
// `in` narrows the alternatives of a lone `{ id } & (A | B)`.
type Ok = { result: number };
type Err = { error: string };
type Message = { id: number } & (Err | Ok);

function read(m: Message): number {
  if ("error" in m) {
    return m.error.length;
  }
  return m.result + m.id;
}

read({ id: 1, result: 2 }) + read({ id: 0, error: "ab" });

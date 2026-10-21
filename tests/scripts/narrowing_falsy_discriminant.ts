// expect: boom
// `!x.ok` narrows a union discriminated by a literal `ok`.
interface Ok {
  ok: true;
  data: string;
}
interface Fail {
  ok: false;
  error: string;
}
function read(r: Ok | Fail): string {
  if (!r.ok) {
    return r.error;
  }
  return r.data;
}
read({ ok: false, error: "boom" });

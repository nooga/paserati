// expect: done:end
function step(it: Iterator<number, string>): string {
  const r = it.next();
  if (r.done) {
    const s: string = r.value;
    return "done:" + s;
  }
  const n: number = r.value;
  return "value:" + n;
}
const it: Iterator<number, string> = { next: () => ({ done: true, value: "end" }) };
step(it);

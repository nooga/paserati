// expect: 9
// map infers its result from the callback; a tuple rest parameter spreads
// into Parameters<>; a returned [] drops out beside another array.
type OnErr = (opts: { error: string }) => void;
type Spy = (...args: Parameters<OnErr>) => void;
const calls: Parameters<Spy>[] = [[{ error: "abc" }]];
const lengths: number[] = calls.map((it) => it[0].error.length);

function parse(ok: boolean) {
  if (ok) {
    return [1, 2].map((x) => String(x * 2));
  }
  return [];
}
const parsed: string[] = parse(true);
const mixed = Math.random() > 2 ? [1] : ["ab", "cd"];
const total = mixed.map((m) => String(m).length).reduce((a, b) => a + b, 0);

lengths[0] + parsed.length + total + parse(false).length;

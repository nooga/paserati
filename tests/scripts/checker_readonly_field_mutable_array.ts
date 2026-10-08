// expect: a|1,2
// A readonly field holding a mutable array: the field can't be reassigned,
// but the array it holds is an ordinary T[].
class P {
  readonly tags: string[] = [];
  constructor(readonly ids: number[]) {}
}
const p = new P([1]);
p.tags.push("a");
p.ids.push(2);
const t: string[] = p.tags;
const i: number[] = p.ids;
`${t.join(",")}|${i.join(",")}`;

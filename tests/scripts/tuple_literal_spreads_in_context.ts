// Array literals with spreads against tuple types: a fixed-length tuple
// spread fills positions, other spreads go to the rest, and the elements
// keep their contextual types (trpc's jsonl encoder).
// expect: 3
type Kind = 0 | 1;
type Chunk = [key: null | string, kind: Kind, id: number];
type Encoded = [[unknown] | [], ...Chunk[]];
function encode(v: unknown, reg: [Kind, number], chunks: Chunk[]): Encoded {
  if (v === 1) return [[v], [null, ...reg]];
  if (v === 2) return [[], ...chunks];
  return [[v], ["k", ...reg], ...chunks];
}
encode(3, [0, 1], [["a", 1, 2]]).length;

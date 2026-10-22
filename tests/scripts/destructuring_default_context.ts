// expect: 7
type Deserialize = (value: any) => any;
function f(opts: { deserialize?: Deserialize; n?: (x: number) => string }) {
  const { deserialize = (v) => v } = opts;
  const { n = (x) => x.toFixed() } = opts;
  return deserialize(3) + n(4).length;
}
const [h = (q) => q + 1]: [((q: number) => number)?] = [];
f({}) + h(2);

// expect: 3
// A type parameter narrowed by `in` (T & { error: unknown }), a generic
// mapped or indexed operand, and an interface with inherited members are
// all spreadable object types.
type Item = { error: string } | { result: number };

function transform<T extends Item>(item: T): number {
  if ("error" in item) {
    const copy = { ...item, extra: 1 };
    return copy.extra;
  }
  return 0;
}

function pickAll<T extends object>(value: { [K in keyof T]: T[K] }, key: T[keyof T]) {
  return { ...value, key };
}

interface Base {
  a: number;
}
interface Derived extends Base {
  b: string;
}
declare const d: Derived;
function readDerived(): number {
  const spread = { ...d };
  return spread.a + spread.b.length;
}

const picked = pickAll<{ n: number }>({ n: 1 }, 1);
transform({ error: "x" }) + picked.key + 1;

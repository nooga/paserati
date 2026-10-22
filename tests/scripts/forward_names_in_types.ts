// expect: 7
// Names a type position may use before their declaration: a namespace merged
// with an earlier interface, a local alias named by a hoisted function, and a
// block-scoped variable in a type query.
interface Schema<I = unknown> {
  readonly props: Schema.Props<I>;
}
declare namespace Schema {
  interface Props<I = unknown> {
    readonly value: I;
  }
}
const schema: Schema<number> = { props: { value: 3 } };

const make = () => {
  type Pair = { a: number; b: number };
  function pair(): Pair {
    return { a: 1, b: 2 };
  }
  return pair();
};

function counter() {
  const seen = new Map<typeof next, string>();
  let next = 1;
  seen.set(next, "one");
  return seen.size;
}

const p = make();
schema.props.value + p.a + p.b + counter();

// expect: value
// Test: type predicate narrowing on generic unions with TypeParam members
// When isShape(spec) narrows T | ObjType<T> to ObjType<unknown>,
// the TypeParam T should be eliminated.

interface ValueNode<T> {
  kind: "value";
  get?: () => T;
  value?: T;
}

type ValueGetter<T> = () => T;
type ValueSpec<T> = T | ValueGetter<T> | ValueNode<T>;

function isValueShape(input: unknown): input is ValueNode<unknown> {
  return typeof input === "object" && input !== null && (input as any).kind === "value";
}

function processValue<T>(spec: ValueSpec<T>): string {
  if (typeof spec === "function") {
    return "getter";
  }
  // typeof eliminates ValueGetter<T>, remaining: T | ValueNode<T>
  if (isValueShape(spec)) {
    // Narrowed to ValueNode<unknown>
    if (typeof spec.get === "function") {
      return "value-getter";
    }
    return "value";
  }
  return "raw";
}

processValue<number>({ kind: "value", value: 42 });

// expect: 3
// instanceof narrows a property reference, not just an identifier.
class Box {
  constructor(public n: number) {}
  size() { return this.n; }
}
type Dict = { [k: string]: number | undefined };
function total(meta: { items?: Box | Dict } | undefined): number {
  if (meta?.items) {
    if (meta.items instanceof Box) {
      return meta.items.size();
    }
    return meta.items["a"] ?? 0;
  }
  return 0;
}
total({ items: new Box(2) }) + total({ items: { a: 1 } }) + total(undefined);

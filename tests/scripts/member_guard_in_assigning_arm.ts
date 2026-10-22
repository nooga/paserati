// expect: 2
// `if (this.x === null) throw` inside a branch arm that also assigns this.x
// still narrows it for the rest of the arm.
class Subs {
  items: readonly number[] | null = [];
  add(flag: boolean, n: number): number {
    if (flag) {
      if (this.items === null) {
        throw new Error("settled");
      }
      this.items = [...this.items, n];
    }
    return this.items?.length ?? 0;
  }
}
const s = new Subs();
s.add(true, 1);
s.add(true, 2);

// expect: 3
type Op = { id: number; path: string };
function g({ op: { id } }: { op: Pick<Op, "id"> }): number { return id; }
const { op: { path } }: { op: Readonly<Op> } = { op: { id: 2, path: "p" } };
const n: number = g({ op: { id: 2 } });
n + path.length;

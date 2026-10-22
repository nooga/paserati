// expect: 7
type Shape = { kind: "sq"; side: number } | { kind: "rect"; w: number; h: number };
function area(s: Shape): number {
  const { kind } = s;
  if (kind === "sq") return s.side * s.side;
  return s.w * s.h;
}
function settled(r: PromiseSettledResult<number>): number {
  const { status } = r;
  if (status === "fulfilled") return r.value;
  return -1;
}
area({ kind: "sq", side: 2 }) + settled({ status: "fulfilled", value: 3 });

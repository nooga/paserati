// expect: true|1|a,b
// #637: readonly T[] and ReadonlyArray<T> expose the non-mutating Array methods.
class E { static readonly FIELDS: readonly string[] = ["password"]; }
interface P { windows: ReadonlyArray<{ day: string }> }
const p: P = { windows: [{ day: "MON" }] };
const xs: readonly string[] = ["b", "a"];
`${E.FIELDS.some((f) => f === "password")}|${p.windows.map((w) => ({ d: w.day })).length}|${[...xs].sort().join(",")}`;

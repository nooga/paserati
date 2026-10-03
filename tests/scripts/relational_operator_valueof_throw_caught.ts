// An exception thrown by valueOf during a relational operator inside a
// called function is caught by the caller's try/catch.
// no-typecheck
// expect: <:TypeError,<=:TypeError,>:TypeError,>=:TypeError
const o: any = { valueOf() { throw new TypeError("x"); } };
const log: string[] = [];
const ops: [string, () => unknown][] = [["<", () => o < o], ["<=", () => o <= o], [">", () => o > o], [">=", () => o >= o]];
for (const [name, f] of ops) {
  try { f(); log.push(name + ":none"); } catch (e) { log.push(name + ":" + (e as Error).name); }
}
log.join(",");

// Await always takes a tick (even for a non-promise), and awaiting a
// thenable calls its then() in a job of its own.
// expect: a,sync,b,p1,then,p2,c,p3
const log: string[] = [];
async function f() {
  log.push("a");
  await 1;
  log.push("b");
  await { then(r: (v: number) => void) { log.push("then"); r(5); } };
  log.push("c");
}
f();
log.push("sync");
await Promise.resolve(0)
  .then(() => log.push("p1"))
  .then(() => log.push("p2"))
  .then(() => log.push("p3"));
log.join(",");

// expect: caught,caught
// A Proxy trap ('has' for the `in` operator, 'deleteProperty' for `delete`)
// that throws must have its exception catchable by a surrounding try/catch.
// Previously these escaped as genuinely-uncaught exceptions because
// frame.ip was never synced before re-throwing the trap's exception, so
// unwindException searched for a handler at a stale PC and missed the real
// one (issue #65 followup: stale frame.ip, not the swallow bug itself).
let results: string[] = [];

const p1 = new Proxy({}, { has() { throw new TypeError("nope"); } });
let r1 = "not caught";
try {
  "a" in p1;
} catch (e) {
  r1 = "caught";
}
results.push(r1);

const p2 = new Proxy({}, { deleteProperty() { throw new TypeError("nope"); } });
let r2 = "not caught";
try {
  delete p2.x;
} catch (e) {
  r2 = "caught";
}
results.push(r2);

results.join(",");

// expect: 3
// A truthiness check removes every definitely falsy member: void as well as
// null and undefined, and boolean narrows to true.
interface Unsub { unsubscribe(): number }
type Teardown = Unsub | (() => number) | void;
function run(t: Teardown | null): number {
  if (typeof t === "function") {
    return t();
  } else if (t) {
    return t.unsubscribe();
  }
  return 0;
}
function flag(b: boolean | string): true | string {
  if (b) {
    return b;
  }
  return "no";
}
const r = run(() => 1) + run({ unsubscribe: () => 2 }) + run(undefined);
flag(true) === true ? r : -1;

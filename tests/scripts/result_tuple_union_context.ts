// expect: 3
// An array literal against a union of same-length tuples is a tuple
// (trpc's ResultTuple), a parameterless arrow takes its contextual return
// type, and a callback's return is typed by what the call is expected to give.
class Failure {
  code = 1;
}
type Result<T> = [undefined, T] | [Failure, undefined];
function run<T>(f: () => T): T {
  return f();
}
let result: Result<number> | undefined;
result = [undefined, 1];
const make: () => [number, string] = () => [2, "x"];
const viaRun: Result<string> = run(() => {
  try {
    return [undefined, "ok"];
  } catch {
    return [new Failure(), undefined];
  }
});
const fails: Result<number> = [new Failure(), undefined];
(result[1] ?? 0) + make()[0] + (viaRun[1] === "ok" ? 0 : 9) + (fails[0] ? 0 : 9);

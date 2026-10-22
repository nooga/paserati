// expect: 0
// An await operand is contextually typed by T | PromiseLike<T>, so a
// literal returned through a generic call becomes the expected tuple; a
// nested function's parameter default is typed by its declared type.
type Pair = [undefined, number] | [Error, undefined];
const run = <V>(fn: () => V): V => fn();
function outer() {
  function step(path: readonly string[] = []) {
    return path.length;
  }
  return step();
}
async function main(): Promise<number> {
  const pair: Pair = await run(async () => [undefined, 2]);
  return (pair[1] ?? 0) + outer();
}
main();
outer();

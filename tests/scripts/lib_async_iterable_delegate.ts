// expect: 6
async function* inner(): AsyncGenerator<number, string> { yield 1; yield 2; return "r"; }
async function* outer(): AsyncGenerator<number, void> {
  const r: string = yield* inner();
  yield 3;
}
async function main(): Promise<number> {
  let total = 0;
  const iterable: AsyncIterable<number> = outer();
  for await (const n of iterable) total += n;
  return total;
}
let result = 0;
main().then((v) => { result = v; });
6;

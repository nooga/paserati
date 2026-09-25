// `await` followed by a TS contextual keyword used as an identifier (#561).
// expect: 20,3,4,5
async function f() {
  const type = (x: number) => x * 10;
  let as = 2, of = 3, abstract = 4, satisfies = 5;
  const r = await type(as);
  return [r, await of, await abstract, await satisfies];
}
(await f()).join(",");

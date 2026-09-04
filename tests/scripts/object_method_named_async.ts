// expect: 10
// #229: a shorthand method literally named 'async' - { async(...) {} } - was
// always parsed as the async-modifier form, which requires a *separate*
// PropertyName before '(' and so failed whenever 'async' was immediately
// followed by '('. Per the grammar, AsyncMethod is
// `async [no LineTerminator here] PropertyName (` - 'async' directly
// followed by '(' can't be that production at all, so it must be a plain
// method named 'async' instead (confirmed against real Node).
const o = {
  async(...args: number[]) { return args.reduce((a, b) => a + b, 0); },
};
const total = o.async(1, 2, 3, 4); // 10

// Sibling forms that must keep working alongside the fix above.
const p = {
  async async() { return 5; }, // async method literally named 'async'
  async *gen() { yield 1; yield 2; }, // async generator - regression trap
};
p.async().then((v: number) => {
  if (v !== 5) throw new Error("async-named-async method returned " + v);
});
(async () => {
  let genTotal = 0;
  for await (const v of p.gen()) genTotal += v;
  if (genTotal !== 3) throw new Error("async generator returned wrong total: " + genTotal);
})();

const g = { get async() { return 7; }, set async(v: number) { /* noop */ } };
if (g.async !== 7) throw new Error("getter named async failed: " + g.async);
g.async = 9;

total;

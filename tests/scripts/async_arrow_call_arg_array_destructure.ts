// Regression test for #477: async arrow function with an array-destructured
// second parameter and a block body, passed directly as a call argument,
// must parse correctly (contextual keywords like `from` must be usable as
// destructuring targets in parameter position).
// expect: 10

const arr: [number, number][] = [[1, 2], [3, 4]];
const result = arr.reduce(async (prev: Promise<number>, [from, value]: [number, number]) => {
  const acc = await prev;
  return acc + from + value;
}, Promise.resolve(0));

await result;

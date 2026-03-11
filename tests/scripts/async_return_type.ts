// expect: true
// Bug 1: async function return type should unwrap Promise<T> for return value checking
async function foo(): Promise<{ done: boolean }> {
  return { done: true };
}

async function main() {
  const result = await foo();
  return result.done;
}

const r = await main();
r;

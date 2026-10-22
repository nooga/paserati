// expect: 5
// An async function's returned `T | Promise<T>` is awaited member by
// member, so it satisfies Promise<T> (tRPC's createContext wrappers).
function wrap<T>(create: () => T | Promise<T>): () => Promise<T> {
  const run: () => Promise<T> = async () => {
    return create();
  };
  return run;
}

async function direct<T>(create: () => T | Promise<T>): Promise<T> {
  return create();
}

const fromValue = wrap(() => 2);
const fromPromise = direct(() => Promise.resolve(3));
typeof fromValue === "function" && fromPromise instanceof Promise ? 5 : 0;

// Promise.all resolves each element through C.resolve and Invoke(then) as it
// iterates (so a patched then is observed), resolving with a promise takes
// the thenable-job route, and Array.fromAsync awaits an async generator.
// no-typecheck
// expect: 3|then-calls:2|any:AggregateError:2|from:1,2,3|withResolvers:ok
const out: string[] = [];

const all = await Promise.all([1, Promise.resolve(2)]);
out.push(String(all[0] + all[1]));

let thenCalls = 0;
const origThen = Promise.prototype.then;
Promise.prototype.then = function (a, b) {
  thenCalls++;
  return origThen.call(this, a, b);
};
const p = Promise.all([1, 2]);
Promise.prototype.then = origThen;
await p;
out.push("then-calls:" + thenCalls);

try {
  await Promise.any([Promise.reject(1), Promise.reject(2)]);
} catch (e) {
  out.push("any:" + e.name + ":" + e.errors.length);
}

async function* g() {
  yield 1;
  yield Promise.resolve(2);
  yield 3;
}
out.push("from:" + (await Array.fromAsync(g())).join(","));

const { promise, resolve } = Promise.withResolvers();
resolve("ok");
out.push("withResolvers:" + (await promise));

out.join("|");

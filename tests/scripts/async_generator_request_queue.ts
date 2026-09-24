// Async generator requests queue up while the body is awaiting, each gets
// its own promise, `yield` awaits its operand, and return() runs finally
// blocks (which may themselves await).
// expect: 1,2,rejected:boom,done:true,finally:after-await,r:7
const out: string[] = [];

async function* g() {
  yield 1;
  await null;
  yield Promise.resolve(2);
  yield Promise.reject("boom");
}
const it = g();
const [a, b, c, d] = [it.next(), it.next(), it.next(), it.next()];
out.push(String((await a).value));
out.push(String((await b).value));
try {
  await c;
} catch (e) {
  out.push("rejected:" + e);
}
out.push("done:" + (await d).done);

async function* h() {
  try {
    yield 1;
  } finally {
    await null;
    out.push("finally:after-await");
  }
}
const it2 = h();
await it2.next();
const r = await it2.return(Promise.resolve(7));
out.push("r:" + r.value);
out.join(",");

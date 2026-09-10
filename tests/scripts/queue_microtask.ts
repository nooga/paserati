// expect: true
// #393: queueMicrotask() didn't exist as a global at all, which meant the
// very common pattern `queueMicrotask(() => controller.close())` inside a
// ReadableStream pull() threw "queueMicrotask is not defined" - undici's
// extractBody() uses exactly this to close its request-body stream.
const results: unknown[] = [];

results.push(typeof queueMicrotask === "function");

async function main(): Promise<void> {
  const order: string[] = [];
  order.push("sync-start");
  queueMicrotask(() => {
    order.push("micro-1");
  });
  Promise.resolve(undefined).then(() => {
    order.push("then");
  });
  queueMicrotask(() => {
    order.push("micro-2");
  });
  order.push("sync-end");

  await Promise.resolve(undefined);
  await Promise.resolve(undefined);

  // Both queued microtasks (and the promise reaction) must run after the
  // synchronous code, and queueMicrotask callbacks must run in FIFO order.
  results.push(order.join(",") === "sync-start,sync-end,micro-1,then,micro-2");
}
await main();

// A non-callable argument throws synchronously, per spec.
try {
  (queueMicrotask as any)(42);
  results.push(false);
} catch (e) {
  results.push(true);
}

results.every((r) => r === true);

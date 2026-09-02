// expect: true
// #205: a minimal ReadableStream/ReadableStreamDefaultReader, sized to what
// real streaming SDK consumers need: stream[Symbol.asyncIterator] and
// stream.getReader() -> { read(), releaseLock(), cancel() }.
const results: unknown[] = [];

async function main(): Promise<boolean> {
  // getReader()/read()/releaseLock(), plus a pull-based source (backpressure).
  let pullCount = 0;
  const pulled = new ReadableStream({
    pull(controller: any) {
      pullCount++;
      if (pullCount <= 2) controller.enqueue(pullCount);
      else controller.close();
    },
  });
  const reader = pulled.getReader();
  const r1 = await reader.read();
  const r2 = await reader.read();
  const r3 = await reader.read();
  reader.releaseLock();
  results.push(r1.value === 1 && r1.done === false);
  results.push(r2.value === 2 && r2.done === false);
  results.push(r3.value === undefined && r3.done === true);

  // [Symbol.asyncIterator] / for-await-of.
  const iterable = new ReadableStream({
    start(controller: any) {
      controller.enqueue("a");
      controller.enqueue("b");
      controller.close();
    },
  });
  const collected: string[] = [];
  for await (const chunk of iterable as any) {
    collected.push(chunk);
  }
  results.push(collected.length === 2 && collected[0] === "a" && collected[1] === "b");

  // locked / getReader() single-lock enforcement.
  const lockable = new ReadableStream({ start(c: any) { c.enqueue("x"); } });
  results.push(lockable.locked === false);
  const r = lockable.getReader();
  results.push(lockable.locked === true);
  let threw = false;
  try {
    lockable.getReader();
  } catch {
    threw = true;
  }
  results.push(threw);
  r.releaseLock();
  results.push(lockable.locked === false);

  // cancel() forwards to the underlying source and discards the buffered
  // queue (a cancelled stream must not keep handing out old chunks).
  let cancelReason = "";
  const cancellable = new ReadableStream({
    start(c: any) { c.enqueue("buffered"); },
    cancel(reason: any) { cancelReason = reason; },
  });
  const cReader = cancellable.getReader();
  await cReader.cancel("bye");
  const afterCancel = await cReader.read();
  results.push(cancelReason === "bye");
  results.push(afterCancel.done === true);

  // controller.error() rejects reads.
  const erroring = new ReadableStream({
    start(controller: any) {
      controller.enqueue("first");
      controller.error(new Error("boom"));
    },
  });
  const eReader = erroring.getReader();
  const first = await eReader.read();
  results.push(first.value === "first" && first.done === false);
  let rejected = false;
  try {
    await eReader.read();
  } catch (e) {
    rejected = (e as any).message === "boom";
  }
  results.push(rejected);

  return results.every((v) => v === true);
}

await main();

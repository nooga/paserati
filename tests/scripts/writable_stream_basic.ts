// expect: true
// #413: a minimal WritableStream/WritableStreamDefaultWriter, sized to what
// TransformStream.writable (and real streaming SDK sink consumers) need:
// getWriter() -> { write(), close(), abort(), releaseLock(), ready, closed,
// desiredSize }.
const results: unknown[] = [];

async function main(): Promise<boolean> {
  // Basic write()/close(), with a start() and controller.error() left unused.
  const written: unknown[] = [];
  let closedCalled = false;
  const sink = new WritableStream({
    write(chunk: any) {
      written.push(chunk);
    },
    close() {
      closedCalled = true;
    },
  });
  const writer = sink.getWriter();
  await writer.write("a");
  await writer.write("b");
  await writer.close();
  results.push(written.length === 2 && written[0] === "a" && written[1] === "b");
  results.push(closedCalled === true);

  // locked / getWriter() single-lock enforcement.
  const lockable = new WritableStream({ write() {} });
  results.push(lockable.locked === false);
  const w1 = lockable.getWriter();
  results.push(lockable.locked === true);
  let threw = false;
  try {
    lockable.getWriter();
  } catch {
    threw = true;
  }
  results.push(threw);
  w1.releaseLock();
  results.push(lockable.locked === false);

  // Writes are processed in order even without awaiting each one.
  const order: unknown[] = [];
  const ordered = new WritableStream({
    write(chunk: any) {
      order.push(chunk);
    },
  });
  const oWriter = ordered.getWriter();
  const p1 = oWriter.write(1);
  const p2 = oWriter.write(2);
  const p3 = oWriter.write(3);
  await Promise.all([p1, p2, p3]);
  results.push(order.length === 3 && order[0] === 1 && order[1] === 2 && order[2] === 3);

  // write() after close() rejects.
  const closed = new WritableStream({ write() {} });
  const cWriter = closed.getWriter();
  await cWriter.close();
  let rejectedAfterClose = false;
  try {
    await cWriter.write("late");
  } catch {
    rejectedAfterClose = true;
  }
  results.push(rejectedAfterClose);

  // abort() forwards the reason to the sink and rejects writer.closed.
  let abortReason = "";
  const abortable = new WritableStream({
    write() {},
    abort(reason: any) {
      abortReason = reason;
    },
  });
  const aWriter = abortable.getWriter();
  await aWriter.abort("bye");
  results.push(abortReason === "bye");
  let closedRejected = false;
  try {
    await aWriter.closed;
  } catch (e) {
    closedRejected = (e as any) === "bye";
  }
  results.push(closedRejected);

  // controller.error() rejects subsequent writes without invoking abort().
  let abortCalledOnError = false;
  const erroring = new WritableStream({
    write(chunk: any, controller: any) {
      if (chunk === "boom") controller.error(new Error("boom"));
    },
    abort() {
      abortCalledOnError = true;
    },
  });
  const eWriter = erroring.getWriter();
  await eWriter.write("boom");
  let rejectedAfterError = false;
  try {
    await eWriter.write("more");
  } catch (e) {
    rejectedAfterError = (e as any).message === "boom";
  }
  results.push(rejectedAfterError);
  results.push(abortCalledOnError === false);

  return results.every((v) => v === true);
}

await main();

// expect: true
// #390: ReadableStream.prototype.tee() - two independent branches fed from
// one shared underlying source, each getting every chunk.
const results: unknown[] = [];

async function main(): Promise<boolean> {
  // Basic fan-out: both branches see every chunk, in order, then close.
  const rs = new ReadableStream({
    start(controller: any) {
      controller.enqueue(1);
      controller.enqueue(2);
      controller.close();
    },
  });
  results.push(typeof rs.tee === "function");
  results.push(rs.locked === false);

  const [a, b] = rs.tee();
  results.push(rs.locked === true);

  const ra = a.getReader();
  const rb = b.getReader();

  const outA: unknown[] = [];
  const outB: unknown[] = [];
  while (true) {
    const { value, done } = await ra.read();
    if (done) break;
    outA.push(value);
  }
  while (true) {
    const { value, done } = await rb.read();
    if (done) break;
    outB.push(value);
  }
  results.push(outA.length === 2 && outA[0] === 1 && outA[1] === 2);
  results.push(outB.length === 2 && outB[0] === 1 && outB[1] === 2);

  // Cancelling one branch doesn't cancel the shared original; cancelling
  // both does, with a composite [reason1, reason2] reason.
  let cancelledWith: unknown = null;
  const rs2 = new ReadableStream({
    start(controller: any) {
      controller.enqueue("x");
    },
    cancel(reason: any) {
      cancelledWith = reason;
    },
  });
  const [c, d] = rs2.tee();
  const rc = c.getReader();
  const rd = d.getReader();
  await rc.cancel("reasonC");
  results.push(cancelledWith === null);
  const stillReadable = await rd.read();
  results.push(stillReadable.value === "x" && stillReadable.done === false);
  await rd.cancel("reasonD");
  const composite = cancelledWith as unknown[];
  results.push(Array.isArray(composite) && composite[0] === "reasonC" && composite[1] === "reasonD");

  // controller.error() rejects reads on both branches.
  const rs3 = new ReadableStream({
    start(controller: any) {
      controller.enqueue("first");
      controller.error(new Error("boom"));
    },
  });
  const [e, f] = rs3.tee();
  const re = e.getReader();
  const rf = f.getReader();
  results.push((await re.read()).value === "first");
  let eRejected = false;
  let fRejected = false;
  try {
    await re.read();
  } catch (err) {
    eRejected = (err as any).message === "boom";
  }
  try {
    await rf.read();
    await rf.read();
  } catch (err) {
    fRejected = (err as any).message === "boom";
  }
  results.push(eRejected);
  results.push(fRejected);

  // tee() rejects an already-locked stream, same as getReader() does; and
  // a stream teed once cannot be teed again.
  const rs4 = new ReadableStream({ start(c: any) { c.enqueue("y"); } });
  const lockedReader = rs4.getReader();
  let teeThrewOnLocked = false;
  try {
    rs4.tee();
  } catch {
    teeThrewOnLocked = true;
  }
  results.push(teeThrewOnLocked);
  lockedReader.releaseLock();

  const rs5 = new ReadableStream({ start(c: any) { c.enqueue("z"); } });
  rs5.tee();
  let teeThrewOnRetee = false;
  try {
    rs5.tee();
  } catch {
    teeThrewOnRetee = true;
  }
  results.push(teeThrewOnRetee);

  return results.every((v) => v === true);
}

await main();

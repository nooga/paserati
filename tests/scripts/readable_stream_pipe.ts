// expect: true
// #413: ReadableStream.prototype.pipeTo/pipeThrough - needed once
// TransformStream exists, since real SDK code chains
// `source.pipeThrough(transform)` (see transform_stream_basic.ts).
const results: unknown[] = [];

async function main(): Promise<boolean> {
  // pipeTo(): drains this stream into a WritableStream, closing it at the end.
  const source = new ReadableStream({
    start(c: any) {
      c.enqueue(1);
      c.enqueue(2);
      c.enqueue(3);
      c.close();
    },
  });
  const collected: unknown[] = [];
  let destClosed = false;
  const dest = new WritableStream({
    write(chunk: any) {
      collected.push(chunk);
    },
    close() {
      destClosed = true;
    },
  });
  await source.pipeTo(dest);
  results.push(collected.length === 3 && collected[0] === 1 && collected[1] === 2 && collected[2] === 3);
  results.push(destClosed === true);

  // pipeTo() propagates a write failure back by cancelling the source.
  // (#414: this exact shape - a rejection whose value came from a vm.Call
  // throwing inside a native reaction, then awaited/caught at the top level
  // - used to panic under this suite's strict register-window checks with a
  // spurious "register window imbalance ... popTopLevelScriptFrame". Fixed
  // by reclaiming the register directory's cursor in
  // executeUserFunctionSafe/executeUserFunctionWithNewTarget's error paths,
  // the same way Interpret's own nested-call error path already did.)
  let cancelled = false;
  const source2 = new ReadableStream({
    start(c: any) {
      c.enqueue("a");
      c.enqueue("b");
    },
    cancel() {
      cancelled = true;
    },
  });
  const failingDest = new WritableStream({
    write(chunk: any) {
      if (chunk === "b") throw new Error("write failed");
    },
  });
  let pipeRejected = false;
  try {
    await source2.pipeTo(failingDest);
  } catch (e) {
    pipeRejected = (e as any).message === "write failed";
  }
  results.push(pipeRejected);
  results.push(cancelled === true);

  // pipeThrough({readable, writable}): pipes into `writable`, returns
  // `readable` - works for any duck-typed pair, not just a real
  // TransformStream (that combination is covered by transform_stream_basic.ts).
  const upperSource = new ReadableStream({
    start(c: any) {
      c.enqueue("x");
      c.enqueue("y");
      c.close();
    },
  });
  const manual: unknown[] = [];
  let resolvePipeDone: (v: unknown) => void = () => {};
  const pipeDone = new Promise<unknown>((resolve) => {
    resolvePipeDone = resolve;
  });
  const manualWritable = new WritableStream({
    write(chunk: any) {
      manual.push(String(chunk).toUpperCase());
    },
    close() {
      resolvePipeDone(undefined);
    },
  });
  const manualReadable = new ReadableStream({
    start(c: any) {
      c.enqueue("already-here");
      c.close();
    },
  });
  const returned = upperSource.pipeThrough({ readable: manualReadable, writable: manualWritable }) as any;
  results.push(returned === manualReadable);
  const out: unknown[] = [];
  for await (const chunk of returned) out.push(chunk);
  results.push(out.length === 1 && out[0] === "already-here");
  // Wait for the background pipe (upperSource -> manualWritable) to finish -
  // pipeThrough() doesn't expose that pipe's own completion promise (matching
  // spec), so this test observes it indirectly via manualWritable.close().
  await pipeDone;
  results.push(manual.length === 2 && manual[0] === "X" && manual[1] === "Y");

  return results.every((v) => v === true);
}

await main();

// expect: true
// #413: TransformStream joins a ReadableStream and a WritableStream through
// a Transformer's transform()/flush() - the shape real, unmodified SDK code
// (e.g. AWS SDK's @smithy/core checksum stream, its middleware-websocket
// package's `class X extends TransformStream`) actually reaches for.
const results: unknown[] = [];

async function main(): Promise<boolean> {
  // Basic transform(): uppercases each chunk written.
  const upper = new TransformStream({
    transform(chunk: any, controller: any) {
      controller.enqueue(String(chunk).toUpperCase());
    },
  });
  const uWriter = upper.writable.getWriter();
  uWriter.write("hello");
  uWriter.write("world");
  uWriter.close();
  const collected: unknown[] = [];
  for await (const chunk of upper.readable as any) {
    collected.push(chunk);
  }
  results.push(collected.length === 2 && collected[0] === "HELLO" && collected[1] === "WORLD");

  // controller.desiredSize is a live accessor (highWaterMark(1) minus the
  // readable side's current queue length), not a fixed placeholder: it
  // drops once a chunk is enqueued-but-not-yet-read, and recovers once it's
  // read.
  let sizeDuringTransform: unknown = null;
  const sized = new TransformStream({
    transform(chunk: any, controller: any) {
      controller.enqueue(chunk);
      sizeDuringTransform = controller.desiredSize;
    },
  });
  const sizedWriter = sized.writable.getWriter();
  const sizedReader = sized.readable.getReader();
  await sizedWriter.write("a");
  results.push(sizeDuringTransform === 0);
  await sizedReader.read();
  results.push(sizeDuringTransform === 0); // snapshotted during transform(), doesn't retroactively change

  // No transform() given: default algorithm passes chunks through unchanged.
  const passthrough = new TransformStream();
  const pWriter = passthrough.writable.getWriter();
  pWriter.write(1);
  pWriter.write(2);
  pWriter.close();
  const passed: unknown[] = [];
  for await (const chunk of passthrough.readable as any) {
    passed.push(chunk);
  }
  results.push(passed.length === 2 && passed[0] === 1 && passed[1] === 2);

  // flush(): runs once, after the last write, before the readable side closes.
  const withFlush = new TransformStream({
    transform(chunk: any, controller: any) {
      controller.enqueue(chunk);
    },
    flush(controller: any) {
      controller.enqueue("FLUSHED");
    },
  });
  const fWriter = withFlush.writable.getWriter();
  fWriter.write("a");
  await fWriter.close();
  const flushed: unknown[] = [];
  const fReader = withFlush.readable.getReader();
  let fr = await fReader.read();
  while (!fr.done) {
    flushed.push(fr.value);
    fr = await fReader.read();
  }
  results.push(flushed.length === 2 && flushed[0] === "a" && flushed[1] === "FLUSHED");

  // extends TransformStream + super(transformer) - the issue's own repro
  // shape (subclassing a native constructor with an explicit super() call).
  class Upper extends TransformStream {
    constructor() {
      super({
        transform(chunk: any, controller: any) {
          controller.enqueue(String(chunk).toUpperCase());
        },
      });
    }
  }
  const t = new Upper();
  const writer = (t as any).writable.getWriter();
  writer.write("hello");
  writer.close();
  const reader = (t as any).readable.getReader();
  const result = await reader.read();
  results.push(result.value === "HELLO");

  // ReadableStream.pipeThrough(transformStream): pipes through and returns
  // the transform's readable side.
  const source = new ReadableStream({
    start(c: any) {
      c.enqueue("x");
      c.enqueue("y");
      c.close();
    },
  });
  const piped = new TransformStream({
    transform(chunk: any, controller: any) {
      controller.enqueue(String(chunk).toUpperCase());
    },
  });
  const pipedReadable = source.pipeThrough(piped) as any;
  const pipedOut: unknown[] = [];
  for await (const chunk of pipedReadable) {
    pipedOut.push(chunk);
  }
  results.push(pipedOut.length === 2 && pipedOut[0] === "X" && pipedOut[1] === "Y");

  // Breaking out of a for-await-of early calls the async iterator's
  // return(), which cancels the readable side - and per spec, cancelling a
  // TransformStream's readable side errors its writable side too (the
  // background pipe driving pipeThrough's own write loop then fails and
  // cancels the upstream source in turn). That cascade is intentional; this
  // just pins down that an early break neither hangs nor crashes.
  const breakSource = new ReadableStream({
    start(c: any) {
      c.enqueue("x");
      c.enqueue("y");
      c.enqueue("z");
      c.close();
    },
  });
  const breakTransform = new TransformStream({
    transform(chunk: any, controller: any) {
      controller.enqueue(String(chunk).toUpperCase());
    },
  });
  const breakReadable = breakSource.pipeThrough(breakTransform) as any;
  const breakOut: unknown[] = [];
  for await (const chunk of breakReadable) {
    breakOut.push(chunk);
    if (chunk === "X") break;
  }
  results.push(breakOut.length === 1 && breakOut[0] === "X");

  // controller.error() errors both the readable and writable sides.
  const erroring = new TransformStream({
    transform(chunk: any, controller: any) {
      if (chunk === "boom") controller.error(new Error("boom"));
      else controller.enqueue(chunk);
    },
  });
  const eWriter = erroring.writable.getWriter();
  const eReader = erroring.readable.getReader();
  eWriter.write("ok");
  await eReader.read();
  eWriter.write("boom");
  let readErrored = false;
  try {
    await eReader.read();
  } catch (e) {
    readErrored = (e as any).message === "boom";
  }
  results.push(readErrored);
  let writeErrored = false;
  try {
    await eWriter.write("more");
  } catch (e) {
    writeErrored = (e as any).message === "boom";
  }
  results.push(writeErrored);

  return results.every((v) => v === true);
}

await main();

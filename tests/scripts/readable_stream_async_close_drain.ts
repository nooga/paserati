// expect: true
// #393 (follow-up): a "bytes" ReadableStream whose pull() enqueues a chunk
// and then defers controller.close() to a later microtask - the exact shape
// of undici's real extractBody() (async pull, queueMicrotask(() =>
// controller.close())) - must actually reach `done: true` once that close
// lands, not have pull() invoked again and again forever because the
// consumer's `read()` calls raced ahead of the still-pending close().
//
// This traced back to top-level `await` resuming immediately when the
// awaited promise was already settled, instead of always deferring through
// a microtask hop (per spec) - which let it jump the FIFO line ahead of an
// earlier-queued `queueMicrotask(() => controller.close())` and observe a
// stream that looked "still open" when it was really about to close. See
// tests/scripts/queue_microtask.ts for the narrower ordering-only repro and
// tests/scripts/issue_393_text_encoder_bytelength.ts for the original bug.
const results: unknown[] = [];

async function main(): Promise<boolean> {
  let pullCount = 0;
  const encoder = new TextEncoder();
  const rs = new ReadableStream({
    async pull(controller: any) {
      pullCount++;
      const buffer = encoder.encode("hello world");
      if (buffer.byteLength) {
        controller.enqueue(buffer);
      }
      queueMicrotask(() => {
        controller.close();
      });
    },
    type: "bytes",
  });

  const reader = rs.getReader();
  const chunks: number[] = [];
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    chunks.push(value.length);
    if (chunks.length > 5) break; // safety net against a real runaway
  }

  results.push(pullCount === 1);
  results.push(chunks.length === 1 && chunks[0] === 11);

  // A second read past done:true must stay done, not resurrect pull().
  const again = await reader.read();
  results.push(again.done === true);
  results.push(pullCount === 1);

  return results.every((v) => v === true);
}

await main();

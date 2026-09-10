// expect: true
// #393: TextEncoder.prototype.encode() must return a real Uint8Array, not a
// plain Array. A plain Array's .byteLength is undefined (falsy), so any
// ReadableStream pull() that guards enqueue() on the encoded chunk's own
// .byteLength - e.g. undici's extractBody:
//   if (buffer.byteLength) { controller.enqueue(buffer); }
// - would silently drop the chunk (sync pull) or hang forever (async pull),
// even though every other equivalent guard (.length, a literal, a
// different object's .byteLength, or no guard at all) worked fine.
const results: unknown[] = [];

const encoded = new TextEncoder().encode("hi");
results.push(encoded instanceof Uint8Array);
results.push(encoded.byteLength === 2);
results.push(encoded.length === 2);

// The other branch of undici's guard: an empty body must produce a real,
// zero-length Uint8Array (falsy .byteLength), not a truthy plain object.
const empty = new TextEncoder().encode("");
results.push(empty instanceof Uint8Array);
results.push(empty.byteLength === 0);
results.push(!empty.byteLength);

async function readOne(
  rs: ReadableStream
): Promise<{ done: boolean; length: number }> {
  const { done, value } = await rs.getReader().read();
  return { done, length: value ? value.length : -1 };
}

// Sync pull: used to silently drop the chunk (done: true, value: undefined).
const syncStream = new ReadableStream({
  pull(controller: any) {
    const buffer = new TextEncoder().encode("hi");
    if (buffer.byteLength) {
      controller.enqueue(buffer);
    }
    controller.close();
  },
});
const syncResult = await readOne(syncStream);
results.push(syncResult.done === false && syncResult.length === 2);

// Async pull: used to hang forever (read() promise never settled).
const buffer = new TextEncoder().encode("hi");
const asyncStream = new ReadableStream({
  async pull(controller: any) {
    if (buffer.byteLength) {
      controller.enqueue(buffer);
    }
    await Promise.resolve(undefined);
    controller.close();
  },
});
const asyncResult = await readOne(asyncStream);
results.push(asyncResult.done === false && asyncResult.length === 2);

results.every((r) => r === true);

// expect: 7
// A declaration without type parameters merges into a generic interface
// whose parameters all have defaults (in either order), and a typed array
// written with its buffer argument (Uint8Array<ArrayBuffer>) is still the
// typed array, as @types/node's Buffer declares it.
interface Bytes {
  label(): string;
}
interface Bytes<TBuffer extends ArrayBufferLike = ArrayBufferLike> extends Uint8Array<TBuffer> {
  copyOut(): Bytes<TBuffer>;
}
interface Bytes {
  sum(): number;
}

function inspect(b: Bytes): number {
  const view: Uint8Array<ArrayBuffer> = new Uint8Array(4);
  return b.length + b.byteLength + view.length - b.label().length - b.copyOut().sum();
}

const raw = new Uint8Array([1, 2, 3]);
const bytes = Object.assign(raw, {
  label: () => "abc",
  copyOut: () => bytes,
  sum: () => 0,
}) as unknown as Bytes;
inspect(bytes);

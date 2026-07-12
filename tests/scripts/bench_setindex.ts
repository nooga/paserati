// Array-element-write-heavy micro-benchmark, isolating the OpSetIndex hot path.
// Deterministic; prints a checksum so the work can't be optimized away.
// expect: 155916747
function run(): number {
  const N = 256;
  const ITERS = 200000;
  const arr = new Array(N);
  for (let i = 0; i < N; i++) arr[i] = i >>> 0;

  let x = 0x12345678 >>> 0;
  let acc = 0 >>> 0;
  for (let i = 0; i < ITERS; i++) {
    x = (x ^ (x << 13)) >>> 0;
    x = (x ^ (x >>> 17)) >>> 0;
    x = (x ^ (x << 5)) >>> 0;
    const idx = x & (N - 1);
    const v = (arr[idx] + (x & 0xffff)) >>> 0;
    arr[idx] = (v ^ (v >>> 7) ^ (v << 9)) >>> 0;
    acc = (acc + arr[idx]) >>> 0;
  }
  return acc >>> 0;
}
run();

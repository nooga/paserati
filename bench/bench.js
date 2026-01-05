// A small, deterministic JS micro-benchmark intended to run on both:
// - ./paserati (TypeScript frontend, runs JS by definition)
// - ./gojac   (Goja-based runner)
//
// Notes:
// - Avoids Node.js globals (no process, no require).
// - Prints a final checksum so engines can't trivially "optimize away" the work.
//
// If you want a faster/slower run, adjust ITERS. Keep it stable when comparing.

const ITERS = 5_000_000;

// Simple xorshift32 PRNG; stays within uint32 and is deterministic.
function xorshift32(x) {
  x |= 0;
  x ^= x << 13;
  x ^= x >>> 17;
  x ^= x << 5;
  return x >>> 0;
}

// Mix integer arithmetic, bit ops, and a tiny amount of array traffic.
function run() {
  let x = 0x12345678 >>> 0;
  let acc = 0 >>> 0;

  // Keep this array modest to avoid memory pressure dominating.
  const arr = new Array(256);
  for (let i = 0; i < arr.length; i++) arr[i] = i;

  for (let i = 0; i < ITERS; i++) {
    x = xorshift32(x + i);
    const idx = x & 255;
    // A couple of dependent operations to keep it "real".
    const v = (arr[idx] + (x & 0xffff)) >>> 0;
    arr[idx] = (v ^ (v >>> 7) ^ (v << 9)) >>> 0;
    acc = (acc + arr[idx]) >>> 0;
  }

  // One final scalar fold of the array to make the state observable.
  for (let i = 0; i < arr.length; i++) acc = (acc ^ arr[i]) >>> 0;

  return acc >>> 0;
}

const result = run();
// Print once; in hyperfine we redirect stdout to /dev/null to avoid IO cost.
console.log(String(result));

// Object-heavy micro-benchmark intended to stress inline caches via:
// - repeated monomorphic property get/set (hot sites)
// - a small amount of polymorphism (2-3 stable shapes)
// - method calls via prototype
//
// Runs on both ./paserati and ./gojac (no Node globals).
//
// Note: We intentionally keep the arithmetic boring; the point is object ops.

const N = 80_000; // number of live objects
const ITERS = 6_000_000; // number of operations

function tickA(o) {
  // Reads and writes a few fields to create more IC sites.
  o.x = o.x + 1;
  o.y = o.y + o.x;
  return o.y;
}

function tickB(o) {
  o.x = o.x + 2;
  o.y = o.y + o.x;
  return o.y;
}

function makeA(i) {
  // Stable insertion order for shape.
  return {
    x: i,
    y: i + 1,
    z: i + 2,
    w: i + 3,
    tick: tickA, // property lookup + call
  };
}

function makeB(i) {
  // B has one extra property -> different shape.
  return {
    x: i,
    y: i + 1,
    z: i + 2,
    extra: i + 4,
    tick: tickB,
  };
}

function makeC(i) {
  // Like A, but we add a property later to force a third shape.
  return {
    x: i,
    y: i + 1,
    z: i + 2,
    tick: tickA,
  };
}

function run() {
  const objs = new Array(N);

  // Populate with a mix of shapes; keep it deterministic.
  for (let i = 0; i < N; i++) {
    if (i % 4 === 0) objs[i] = makeB(i);
    else if (i % 4 === 1) objs[i] = makeC(i);
    else objs[i] = makeA(i);
  }

  // Shape transition on the C objects: add a new property after creation.
  for (let i = 1; i < N; i += 4) objs[i].late = i;

  let acc = 0;

  // Phase 1: monomorphic hot loop on the "A" shape only.
  // This should be the happiest path for ICs.
  for (let i = 0; i < ITERS / 2; i++) {
    const idx = (i * 13) % N;
    const o = objs[idx];
    if (o.w === undefined) continue; // filters to A only (B has no w, C has no w)

    // Many repeated accesses at the same sites.
    o.x = o.x + 1;
    o.y = o.y + o.x;
    o.z = o.z + o.y;
    acc += o.tick(o);
    acc += o.z;
  }

  // Phase 2: small polymorphism across 2-3 stable shapes.
  for (let i = ITERS / 2; i < ITERS; i++) {
    const idx = (i * 17) % N;
    const o = objs[idx];

    // Read a few properties; some may be missing depending on shape.
    const extra = o.extra === undefined ? 0 : o.extra;
    const late = o.late === undefined ? 0 : o.late;

    o.x = o.x + 1;
    o.y = o.y + o.x + extra;
    o.z = o.z + late;
    acc += o.tick(o);
    acc += o.y;
  }

  // Final observable fold (avoid bit ops; keep it simple).
  for (let i = 0; i < 2048; i++) {
    const o = objs[i];
    acc += o.x + o.y + o.z;
  }

  // Keep result in uint32-ish range without bitwise mixing.
  // (This avoids bigint/float drift but keeps it portable.)
  const r = acc % 4294967296;
  return r < 0 ? r + 4294967296 : r;
}

console.log(String(run()));

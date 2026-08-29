// expect: 3|131|0,1,2|20|20
// Frame-owned open-upvalue list (github issue #84): closing a returning frame
// walks only that frame's captures (registers AND spill slots), instead of
// scanning a VM-wide list and missing spill captures entirely.
//
// 1. Sibling closures over the same local must share ONE Upvalue, so a write
//    through one is visible through the other after the frame returns.
// 2. A spill-slot capture (forced by >256 locals) that outlives its frame must
//    read the value as of frame return, not a stale slot.
// 3. Per-iteration `let` bindings (OpCloseUpvalue) must each capture their own i.

function siblings(): number {
  let x = 1;
  const get = () => x;
  const inc = () => {
    x++;
  };
  inc();
  inc();
  return get(); // 3
}

function spillCapture(): number {
  // 300 locals -> the tail spills to slots; `target` is among the spilled.
  let v0 = 0, v1 = 1, v2 = 2, v3 = 3, v4 = 4, v5 = 5, v6 = 6, v7 = 7, v8 = 8, v9 = 9;
  let a0 = 0, a1 = 1, a2 = 2, a3 = 3, a4 = 4, a5 = 5, a6 = 6, a7 = 7, a8 = 8, a9 = 9;
  let b0 = 0, b1 = 1, b2 = 2, b3 = 3, b4 = 4, b5 = 5, b6 = 6, b7 = 7, b8 = 8, b9 = 9;
  let c0 = 0, c1 = 1, c2 = 2, c3 = 3, c4 = 4, c5 = 5, c6 = 6, c7 = 7, c8 = 8, c9 = 9;
  let d0 = 0, d1 = 1, d2 = 2, d3 = 3, d4 = 4, d5 = 5, d6 = 6, d7 = 7, d8 = 8, d9 = 9;
  let e0 = 0, e1 = 1, e2 = 2, e3 = 3, e4 = 4, e5 = 5, e6 = 6, e7 = 7, e8 = 8, e9 = 9;
  let f0 = 0, f1 = 1, f2 = 2, f3 = 3, f4 = 4, f5 = 5, f6 = 6, f7 = 7, f8 = 8, f9 = 9;
  let g0 = 0, g1 = 1, g2 = 2, g3 = 3, g4 = 4, g5 = 5, g6 = 6, g7 = 7, g8 = 8, g9 = 9;
  let h0 = 0, h1 = 1, h2 = 2, h3 = 3, h4 = 4, h5 = 5, h6 = 6, h7 = 7, h8 = 8, h9 = 9;
  let i0 = 0, i1 = 1, i2 = 2, i3 = 3, i4 = 4, i5 = 5, i6 = 6, i7 = 7, i8 = 8, i9 = 9;
  let j0 = 0, j1 = 1, j2 = 2, j3 = 3, j4 = 4, j5 = 5, j6 = 6, j7 = 7, j8 = 8, j9 = 9;
  let k0 = 0, k1 = 1, k2 = 2, k3 = 3, k4 = 4, k5 = 5, k6 = 6, k7 = 7, k8 = 8, k9 = 9;
  let l0 = 0, l1 = 1, l2 = 2, l3 = 3, l4 = 4, l5 = 5, l6 = 6, l7 = 7, l8 = 8, l9 = 9;
  let m0 = 0, m1 = 1, m2 = 2, m3 = 3, m4 = 4, m5 = 5, m6 = 6, m7 = 7, m8 = 8, m9 = 9;
  let n0 = 0, n1 = 1, n2 = 2, n3 = 3, n4 = 4, n5 = 5, n6 = 6, n7 = 7, n8 = 8, n9 = 9;
  let o0 = 0, o1 = 1, o2 = 2, o3 = 3, o4 = 4, o5 = 5, o6 = 6, o7 = 7, o8 = 8, o9 = 9;
  let p0 = 0, p1 = 1, p2 = 2, p3 = 3, p4 = 4, p5 = 5, p6 = 6, p7 = 7, p8 = 8, p9 = 9;
  let q0 = 0, q1 = 1, q2 = 2, q3 = 3, q4 = 4, q5 = 5, q6 = 6, q7 = 7, q8 = 8, q9 = 9;
  let r0 = 0, r1 = 1, r2 = 2, r3 = 3, r4 = 4, r5 = 5, r6 = 6, r7 = 7, r8 = 8, r9 = 9;
  let s0 = 0, s1 = 1, s2 = 2, s3 = 3, s4 = 4, s5 = 5, s6 = 6, s7 = 7, s8 = 8, s9 = 9;
  let s10 = 0, s11 = 0, s12 = 0, s13 = 0, s14 = 0, s15 = 0, s16 = 0, s17 = 0, s18 = 0, s19 = 0;
  let s20 = 0, s21 = 0, s22 = 0, s23 = 0, s24 = 0, s25 = 0, s26 = 0, s27 = 0, s28 = 0, s29 = 0;
  let s30 = 0, s31 = 0, s32 = 0, s33 = 0, s34 = 0, s35 = 0, s36 = 0, s37 = 0, s38 = 0, s39 = 0;
  let s40 = 0, s41 = 0, s42 = 0, s43 = 0, s44 = 0, s45 = 0, s46 = 0, s47 = 0, s48 = 0, s49 = 0;
  let s50 = 0, s51 = 0, s52 = 0, s53 = 0, s54 = 0, s55 = 0, s56 = 0, s57 = 0, s58 = 0, s59 = 0;
  let target = 42;
  const f = () => target + v9 + a0 + s59;
  target = 122;
  return f(); // 122 + 9 + 0 + 0 = 131
}

function loopBindings(): string {
  const fns: Array<() => number> = [];
  for (let i = 0; i < 3; i++) fns.push(() => i);
  return fns[0]() + "," + fns[1]() + "," + fns[2]();
}

function noise(n: number): number {
  const a = n;
  return n <= 0 ? 0 : a + noise(n - 1);
}

// A closure captured before the first `yield` must stay live across every
// suspend and be closed only on completion - so after the generator is done
// and the register stack has been churned hard, get() still reads x (== 20 at
// return), not a recycled register.
function genCaptureAcrossChurn(): number {
  function* g() {
    let x = 10;
    let get = () => x;
    yield 1;
    x = 20;
    yield 2;
    return get;
  }
  const it = g();
  it.next();
  it.next();
  const r = it.next();
  let getFn = r.value as () => number;
  let churn = 0;
  for (let i = 0; i < 2000; i++) churn += noise(30);
  return getFn() + (churn - churn); // 20
}

function genCapture(): number {
  function* g() {
    let x = 10;
    const get = () => x;
    yield 1;
    x = 20;
    yield 2;
    return get;
  }
  const it = g();
  it.next();
  it.next();
  const r = it.next();
  return (r.value as () => number)();
}

`${siblings()}|${spillCapture()}|${loopBindings()}|${genCapture()}|${genCaptureAcrossChurn()}`;

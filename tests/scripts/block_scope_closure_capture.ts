// expect: captured|first first first first first first|first|0,10,20,w0,w1,w2 1,2|AA5|1,2,345|d0,d1,s0,s1,c0,c1,c2,t0,x01,f0,t1,x11,f1,a,b,b0,b1,lb,r0,r1,r2 after 142
// skip-typecheck
// paserati#531: a closure over a block-scoped let/const read garbage once the
// block exited. Arrow closures pinned the captured register with a plain Pin,
// which block-exit reclamation drops, so the next statement reused the slot
// (o.k = 1 made `() => e` return o). And a binding in a switch case (or under
// a label) inside a loop was registered only with the switch's pseudo loop
// context, so its upvalue was never closed per iteration and the case
// comparison temp overwrote it on the next pass.
const res = [];
function f() { const o = {}; { let e = "captured"; o.r = () => e; } o.k = 1; return o.r(); }
res.push(f());
const take = (o) => o;
let r1, r2, r3, r4, r5, r6;
if (true) { const first = { name: "first" }; r1 = take({ get: () => first }); const second = 2; }
if (true) { const first = { name: "first" }; r2 = take({ get: () => first }); }
if (true) { const first = { name: "first" }; r3 = take(() => first); const second = 2; }
if (true) { const first = { name: "first" }; const o = { get: () => first }; r4 = o; const second = 2; }
if (true) { const first = { name: "first" }; r5 = [() => first]; const second = 2; }
{ const first = { name: "first" }; r6 = take({ get: () => first }); const second = 2; }
res.push([r1.get()?.name, r2.get()?.name, r3()?.name, r4.get()?.name, r5[0]()?.name, r6.get()?.name].join(" "));
function fn() { let r; if (true) { const first = { name: "first" }; r = take({ get: () => first }); const second = 2; } return r.get()?.name; }
res.push(fn());
const fs0 = [];
for (let i = 0; i < 3; i++) { { let e = i * 10; fs0.push(() => e); } }
let j0 = 0; while (j0 < 3) { { const e = "w" + j0; fs0.push(() => e); } j0++; }
function loopy() { const out = []; for (const k of [1, 2]) { if (k) { let v = k; out.push(() => v); } } return out.map(f => f()).join(); }
res.push(fs0.map(f => f()).join() + " " + loopy());
function g() { const o = {}; { let a = "A"; o.m = function () { return a; }; o.n = { get x() { return a; } }; } o.z = 9; let q = 5; return o.m() + o.n.x + q; }
res.push(g());
function sib() { const o = []; { let a = 1; o.push(() => a); } { let b = 2; o.push(() => b); } { let c = 3; o.push(() => c); } const t = 4, u = 5; return o.map(f => f()).join() + t + u; }
res.push(sib());
const fs = [];
let j = 0;
do { { let e = "d" + j; fs.push(() => e); } j++; } while (j < 2);
for (const k of [0, 1]) { switch (k) { case 0: { let e = "s0"; fs.push(() => e); break; } default: { let e = "s1"; fs.push(() => e); } } }
outer: for (let i = 0; i < 3; i++) { { let e = "c" + i; fs.push(() => e); if (i < 2) continue outer; } }
for (let i = 0; i < 2; i++) { try { let e = "t" + i; fs.push(() => e); throw 1; } catch (err) { let e = "x" + i; fs.push(() => e + err); } finally { let e = "f" + i; fs.push(() => e); } }
for (const key in { a: 1, b: 2 }) { { const e = key; fs.push(() => e); } }
let n = 0; while (true) { if (n >= 2) break; { let e = "b" + n; fs.push(() => e); } n++; }
// labeled block, break out of it
lbl: { let e = "lb"; fs.push(() => e); break lbl; }
const after = "after";
// recursion re-entering the same block
function rec(d, acc) { { let e = "r" + d; acc.push(() => e); } if (d < 2) rec(d + 1, acc); return acc; }
rec(0, fs);
// mutation after capture is visible, and after block exit value is kept
function mut() { let get, set; { let e = 1; get = () => e; set = (v) => { e = v; }; } const z = 100; set(42); return get() + z; }
res.push(fs.map(f => f()).join(",") + " " + after + " " + mut());
res.join("|");

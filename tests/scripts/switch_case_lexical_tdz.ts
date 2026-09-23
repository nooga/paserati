// expect: 1 ; ReferenceError ; 1 ; 2 ; ReferenceError ; ok ; ReferenceError ; 4 ; ReferenceError ; number ; ReferenceError ; ReferenceError ; 0/ReferenceError ; 1 ; 2
// skip-typecheck
// paserati#552: a let/const declared in one switch case is uninitialized
// until its declaration runs, so reading or assigning it from a case entered
// directly throws ReferenceError instead of seeing a stale value. Every entry
// matches Node.
var out = [];
function t(f) { try { return String(f()); } catch (e) { return e.name; } }
function f(n) { switch (n) { case 0: let x = 1; case 1: return x; } }
out.push(t(() => f(0)), t(() => f(1)), t(() => f(0)));
function g(n) { switch (n) { case 0: const c = 2; case 1: return (() => c)(); } }
out.push(t(() => g(0)), t(() => g(1)));
function h(n) { switch (n) { case 0: let y = 1; break; case 1: y = 5; return y; } return "ok"; }
out.push(t(() => h(0)), t(() => h(1)));
function k(n) { switch (n) { case 0: let [a, b] = [1, 2]; case 1: let {z} = {z: 3}; return a + z; } }
out.push(t(() => k(0)), t(() => k(1)));
function d(n) { switch (n) { default: let q = 7; case 1: return typeof q; } }
out.push(t(() => d(9)), t(() => d(1)));
function sel(n) { switch (n) { case w: return "w"; case 1: let w = 1; } }
out.push(t(() => sel(1)));
function loop() { var r = []; for (var i = 0; i < 3; i++) { switch (i) { case 0: let v = i; case 1: r.push(t(() => v)); } } return r.join("/"); }
out.push(loop());
function inner(n) { switch (n) { case 0: let m = 1; return m; case 1: { return 2; } } }
out.push(t(() => inner(0)), t(() => inner(1)));
out.join(" ; ");

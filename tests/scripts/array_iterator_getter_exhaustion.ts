// expect: 1 ; 42 ; 42 ; 1 ; 42 ; 1,42 ; 5 ; 1 ; true,true ; true,true ; true,true ; true,true ; true,true ; true ; 0 ; true,true
// skip-typecheck
// paserati#551: array destructuring reads through an own index accessor
// (the raw-index fast path declines such arrays), and a built-in iterator
// that has reported done stays done even if its source grows. Every entry
// matches Node.
var out = [];
var a = [1, 2]; Object.defineProperty(a, "1", {get() { return 42; }});
var [p, q] = a; out.push(p, q);
var [, r = 7] = a; out.push(r);
var s; [s] = a; out.push(s);
function f([x, y]) { return y; } out.push(f(a));
out.push([...a].join());
var calls = 0; var b = [0]; Object.defineProperty(b, "0", {get() { calls++; return 5; }});
var [u] = b; out.push(u, calls);
function ex(it, grow) { var d1 = it.next().done; grow(); return d1 + "," + it.next().done; }
var arr = []; out.push(ex(arr.keys(), () => arr.push(1)));
var arr2 = []; out.push(ex(arr2.entries(), () => arr2.push(1)));
var ta = new Uint8Array(0); out.push(ex(ta.values(), () => 0));
function g() { return ex(arguments[Symbol.iterator](), () => { arguments[0] = 1; arguments.length = 1; }); } out.push(g());
var al = {length: 0}; var it = Array.prototype.values.call(al); out.push(ex(it, () => { al.length = 1; al[0] = 1; }));
var arr3 = [1]; var it3 = arr3[Symbol.iterator](); for (var v of it3) {} arr3.push(2); out.push(it3.next().done);
var arr4 = [1]; var it4 = arr4.values(); it4.next(); it4.next(); arr4.push(2, 3); out.push([...it4].length);
var m = new Map(); var mi = m.keys(); out.push(ex(mi, () => m.set(1,1)));
out.join(" ; ");

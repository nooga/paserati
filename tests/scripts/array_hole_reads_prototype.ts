// expect: undefined ; undefined ; O ; O5 ; O ; 0,O,2 ; 0|O|2 ; 0=0,1=O,2=2 ; 0,O,2 ; undefined ; undefined ; 0,,2 ; S ; 0,S,0 ; S ; RangeError:boom ; RangeError:boom ; RangeError:boom ; RangeError:boom ; recv ; recv ; recv ; undefined ; ,1 ; g0,O1 ; 0,g0;1,O1 ; p5 ; 100001
// skip-typecheck
// paserati#553: a hole (or an index past the elements) reads through to an
// index property on the prototype chain - for a[i], destructuring, for-of,
// entries(), spread, and array-like iteration - with the array as the
// getter's receiver and a throwing getter propagating. Every entry matches
// Node.
var out = [];
function t(f) { try { return String(f()); } catch (e) { return e.name + ":" + e.message; } }
var a = [0, , 2];
out.push(t(() => a[1]), t(() => a[5]));
Object.prototype[1] = "O"; Object.prototype[5] = "O5";
out.push(t(() => a[1]), t(() => a[5]), t(() => { var [x, y] = a; return y; }), t(() => [...a].join()));
function f(...r) { return r.join("|"); }
out.push(t(() => f(...a)));
var e = []; for (var [k, v] of a.entries()) e.push(k + "=" + v); out.push(e.join());
var fo = []; for (var v2 of a) fo.push(v2); out.push(fo.join());
delete Object.prototype[1]; delete Object.prototype[5];
out.push(t(() => a[1]), t(() => { var [, y] = a; return y; }), t(() => [...a].join()));
class Sub extends Array {} Sub.prototype[1] = "S";
var s = new Sub(0, 0, 0); delete s[1];
out.push(t(() => s[1]), t(() => [...s].join()), t(() => { var [, y] = s; return y; }));
delete Sub.prototype[1];
Object.defineProperty(Array.prototype, "0", { get() { throw new RangeError("boom"); }, configurable: true });
var h = [, 1];
out.push(t(() => h[0]), t(() => { var [z] = h; return z; }), t(() => [...h]), t(() => { for (var q of h) return q; }));
Object.defineProperty(Array.prototype, "0", { get() { return this === h ? "recv" : "other"; }, configurable: true });
out.push(t(() => h[0]), t(() => [...h][0]), t(() => h.values().next().value));
delete Array.prototype[0];
out.push(t(() => h[0]), t(() => [...h].join()));
var al = { length: 2, get 0() { return "g0"; } }; Object.prototype[1] = "O1";
out.push(t(() => [...Array.prototype.values.call(al)].join()), t(() => [...Array.prototype.entries.call(al)].join(";")));
delete Object.prototype[1];
var sp = []; sp[100000] = 1; Array.prototype[5] = "p5";
out.push(t(() => sp[5]), t(() => sp.length));
delete Array.prototype[5];
out.join(" ; ");

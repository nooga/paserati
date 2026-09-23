// expect: 1,2 ; 1 2 ; 2 ; A B ; ga gb ; 9 ; 3 ; 5 5
// skip-typecheck
// paserati#550: an inline-cached prototype-chain read re-checks every
// prototype between the receiver and the holder, so a property that an
// intermediate prototype has (or gains after the cache warmed) shadows
// the holder's. Every entry matches Node.
const out = [];
function read(o) { return o.x; }
// intermediate gains the property after the site is warm
var top = {x: 1}; var middle = Object.create(top); var leaf = Object.create(middle);
var r1 = read(leaf); middle.x = 2; out.push([r1, read(leaf)].join());
// same receiver shape, different intermediates
var m1 = Object.create(top), m2 = Object.create(top); m2.x = 2;
out.push(read(Object.create(m1)) + " " + read(Object.create(m2)));
// receiver's prototype swapped under a warm site
var c = Object.create(m1); read(c); Object.setPrototypeOf(c, m2); out.push(read(c));
// class method added to an intermediate prototype
class A { who() { return "A"; } }
class B extends A {}
class C extends B {}
const inst = new C();
function who(o) { return o.who(); }
var w1 = who(inst); B.prototype.who = function () { return "B"; }; out.push(w1 + " " + who(inst));
// accessor on an intermediate
var gtop = {}; Object.defineProperty(gtop, "g", { get() { return "ga"; } });
var gmid = Object.create(gtop); var gleaf = Object.create(gmid);
function rg(o) { return o.g; }
var g1 = rg(gleaf); Object.defineProperty(gmid, "g", { get() { return "gb"; } }); out.push(g1 + " " + rg(gleaf));
// deeper chain
var d0 = {y: 1}, d1 = Object.create(d0), d2 = Object.create(d1), d3 = Object.create(d2), d4 = Object.create(d3);
function ry(o) { return o.y; }
ry(d4); d2.y = 9; out.push(ry(d4));
// deleting the shadowing property uncovers the holder again
var s = {z: 3}; var smid = Object.create(s); smid.z = 4; var sleaf = Object.create(smid);
function rz(o) { return o.z; }
rz(sleaf); delete smid.z; out.push(rz(sleaf));
// warm reads stay correct on repeat
var t = 0; for (let i = 0; i < 5; i++) t = read(leaf) + read(Object.create(m2)) + 1;
out.push(t + " " + (read(leaf) + read(leaf) + 1));
out.join(" ; ");

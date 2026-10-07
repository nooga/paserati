// expect: static=true | after class | inst=E x=undefined | inst=E x=undefined | caught boom
// #617: non-static method decorator initializers run per instance with this = instance
// (before fields); static ones run at definition with this = class; throws propagate.
const log: string[] = [];
function inst(m: any, c: any) {
  c.addInitializer(function (this: any) { log.push("inst=" + this.constructor.name + " x=" + this.x); });
  return m;
}
function stat(m: any, c: any) {
  c.addInitializer(function (this: any) { log.push("static=" + (this === E)); });
  return m;
}
class E {
  x: number = 1;
  @inst f() {}
  @stat static g() {}
}
log.push("after class");
new E();
new E();
function boom(m: any, c: any) {
  c.addInitializer(function () { throw new Error("boom"); });
  return m;
}
try {
  class F { @boom h() {} }
  new F();
} catch (e: any) {
  log.push("caught " + e.message);
}
log.join(" | ");

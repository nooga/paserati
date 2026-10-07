// expect: 1,true,SyntaxError
// #612: indirect eval only bans super outside methods, not inside class code
const ev: any = eval;
const a = ev("(function(){ class A { m() { return 1 } } class B extends A { m() { return super.m() } } return new B().m() })()");
const b = ev("({ m() { return super.toString === Object.prototype.toString } }).m()");
let c = "";
try { ev("super.x"); } catch (e: any) { c = e.name; }
[a, b, c].join(",");

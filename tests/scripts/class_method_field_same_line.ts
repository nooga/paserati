// expect: pass
var C = class {
  m() { return 42; } a
  b = 10
}

var c = new C();
(c.m() === 42 && c.a === undefined && c.b === 10) ? "pass" : "fail";

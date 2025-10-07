// expect: pass
var C = class {
  x
  y
};

var c = new C();
(c.x === undefined && c.y === undefined) ? "pass" : "fail";

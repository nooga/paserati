// expect: pass
var C = class {
  ["x"]
  ["y"] = 42
}

var c = new C();
(c.x === undefined && c.y === 42) ? "pass" : "fail";

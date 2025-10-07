// expect: pass
var C = class {
  [0] = 10; [1] = 20; [2] = 30
}

var c = new C();
(c[0] === 10 && c[1] === 20 && c[2] === 30) ? "pass" : "fail";

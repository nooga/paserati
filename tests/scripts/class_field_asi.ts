// expect: o
var C = class {
  x = "lol"
  [1]
}

var c = new C();
c.x;

// expect: 42
var obj = {}
var C = class {
  x = obj
  ['lol'] = 42
}

var c = new C();
c.x;

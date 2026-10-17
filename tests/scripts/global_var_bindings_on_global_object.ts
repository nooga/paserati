// Global var/function bindings are data properties of the global object
// (non-configurable for script declarations, configurable for eval-created
// ones and implicit globals); let/const/class bindings are not; a var
// redeclared in eval keeps its value
// no-typecheck
// expect: v:w/e/-|f:w/e/-|ev:w/e/c|ef:w/e/c|iv:w/e/c|zz:w/e/c|let:none|class:none|23,7,function

var v = 1;
function f() {}
let l = 1;
class K {}
eval("var ev = 1; function ef() {}");
(0, eval)("var iv = 1;");
zz = 1;

function attrs(name) {
  var d = Object.getOwnPropertyDescriptor(globalThis, name);
  if (!d) return name + ":none";
  return name + ":" + (d.writable ? "w" : "-") + "/" + (d.enumerable ? "e" : "-") + "/" + (d.configurable ? "c" : "-");
}

var x = 23;
eval("var x;");
var y = 7;
(0, eval)("var y;");
(0, eval)("var f;");

[attrs("v"), attrs("f"), attrs("ev"), attrs("ef"), attrs("iv"), attrs("zz"),
 "let:" + (globalThis.hasOwnProperty("l") ? "own" : "none"),
 "class:" + (globalThis.hasOwnProperty("K") ? "own" : "none"),
 [x, y, typeof f].join(",")].join("|");

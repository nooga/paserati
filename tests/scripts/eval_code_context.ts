// Direct/indirect eval code context: return / new.target / super early
// errors, new.target inheritance, strict eval var environment, field
// initializer new.target.
// expect: true,true,true,true,true,true,undefined,undefined
// no-typecheck

function throwsSyntaxError(fn) {
  try {
    fn();
  } catch (e) {
    return e instanceof SyntaxError;
  }
  return false;
}

function F() {
  return eval("new.target");
}

function G() {
  return (() => eval("new.target"))();
}

function noHome() {
  return eval("super.x");
}

class C {
  x = new.target;
}

(function () {
  "use strict";
  eval("var strictLocal = 1; function strictFn() {}");
})();

[
  throwsSyntaxError(() => eval("return;")),
  throwsSyntaxError(() => eval("new.target")),
  throwsSyntaxError(noHome),
  new F() === F && F() === undefined,
  new G() === G,
  typeof strictLocal === "undefined" && typeof strictFn === "undefined",
  String(new C().x),
  eval('"use strict"; var r = 2; typeof q'),
].join(",");

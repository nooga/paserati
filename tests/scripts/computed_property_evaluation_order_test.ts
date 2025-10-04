// Test computed property name evaluation order
// Per ECMAScript spec, property key must be evaluated and converted to
// property key (calling toString()) BEFORE the property value is evaluated
// expect: pass

let value = "bad";

const key = {
  toString() {
    value = "ok";
    return "p";
  }
};

const obj = {
  [key]: value
};

// key.toString() should be called first (setting value="ok")
// then value is evaluated (getting "ok")
// so obj.p should be "ok", not "bad"
if (obj.p !== "ok") {
  throw new Error(`Expected obj.p to be "ok", got "${obj.p}"`);
}

'pass';

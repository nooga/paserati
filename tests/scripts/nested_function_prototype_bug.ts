// FIXME: Bug - nested function prototype assignment on second call doesn't work correctly
// expect: pass
// When a function defines an inner constructor and assigns .prototype,
// calling the outer function multiple times causes the second call
// to use stale prototype from the first call.

var Proto1 = { foo: "foo" };

function makeInheritor(proto: any) {
  function Inner() {}
  Inner.prototype = proto;
  return new Inner();
}

var obj1 = makeInheritor(Proto1);
var test1 = obj1.foo === "foo";
var test2 = Object.getPrototypeOf(obj1) === Proto1;

// Modify obj1 and use it as prototype for second call
obj1.bar = "bar";
var Proto2 = obj1;

var obj2 = makeInheritor(Proto2);
var test3 = obj2.bar === "bar";
var test4 = Object.getPrototypeOf(obj2) === Proto2;

// All should be true
(test1 && test2 && test3 && test4) ? "pass" : "fail: " + test1 + " " + test2 + " " + test3 + " " + test4

// expect: done
// Test: basic class expression inheritance works (using user-defined parent)
// FIXME: subclassing built-in Error silently fails

class Parent {
  parentField = "parent";
}

var Child = class extends Parent {
  childField = "child";
};

const c = new Child();
c.parentField === "parent" && c.childField === "child" ? "done" : "fail"

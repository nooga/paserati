// Test computed property access on instance
// expect: works

class MyClass {
  normalMethod() {
    return "normal";
  }
}

// Add a computed property to the prototype
(MyClass as any).prototype["computedMethod"] = function () {
  return "computed";
};

const obj = new MyClass();
console.log("normal method:", obj.normalMethod());
console.log("computed method:", obj["computedMethod"]());

("works");

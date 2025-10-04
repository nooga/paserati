// Debug inheritance issue
class Parent {
}

class Child extends Parent {
  foo() {
    return 42;
  }
}

console.log("Child.prototype:", Child.prototype);
console.log("Child.prototype.foo:", Child.prototype.foo);
console.log("Child.prototype.constructor:", Child.prototype.constructor);

const c = new Child();
console.log("Instance c:", c);
console.log("typeof c:", typeof c);
console.log("c.constructor:", c.constructor);

const proto = Object.getPrototypeOf(c);
console.log("Object.getPrototypeOf(c):", proto);
console.log("proto.foo:", proto.foo);
console.log("proto.constructor:", proto.constructor);

console.log("Child.prototype === Object.getPrototypeOf(c):", Child.prototype === Object.getPrototypeOf(c));

console.log("Trying to call c.foo()...");
console.log("c.foo:", c.foo);

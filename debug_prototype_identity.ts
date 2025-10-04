// Check prototype identity
class Parent {
}

class Child extends Parent {
}

const c = new Child();
console.log("Child.prototype:", Child.prototype);
console.log("instance [[Prototype]]:", Object.getPrototypeOf(c));
console.log("Are they the same object?", Child.prototype === Object.getPrototypeOf(c));

// Check if Child.prototype has any properties
Child.prototype.test = "hello";
console.log("Child.prototype.test:", Child.prototype.test);
console.log("instance.test:", c.test);

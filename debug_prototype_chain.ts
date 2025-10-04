// Minimal inheritance test
class Parent {
}

Parent.prototype.parentMethod = function() { return "parent"; };

class Child extends Parent {
}

Child.prototype.childMethod = function() { return "child"; };

console.log("=== Constructor prototypes ===");
console.log("Parent.prototype:", Parent.prototype);
console.log("Child.prototype:", Child.prototype);
console.log("Child.prototype[[Prototype]] (should be Parent.prototype):", Object.getPrototypeOf(Child.prototype));
console.log("Child.prototype[[Prototype]] === Parent.prototype:", Object.getPrototypeOf(Child.prototype) === Parent.prototype);

console.log("\n=== Instance prototype ===");
const c = new Child();
console.log("instance:", c);
const instanceProto = Object.getPrototypeOf(c);
console.log("Object.getPrototypeOf(instance):", instanceProto);
console.log("Object.getPrototypeOf(instance) === Child.prototype:", instanceProto === Child.prototype);

console.log("\n=== Method lookup ===");
console.log("Child.prototype.childMethod:", Child.prototype.childMethod);
console.log("instanceProto.childMethod:", instanceProto.childMethod);
console.log("instance.childMethod:", c.childMethod);

// Check if Child.prototype is set correctly
class Parent {
}

console.log("After Parent defined:");
console.log("  Parent.prototype:", Parent.prototype);

class Child extends Parent {
}

console.log("\nAfter Child defined:");
console.log("  Child.prototype:", Child.prototype);
console.log("  Child.prototype[[Prototype]]:", Object.getPrototypeOf(Child.prototype));
console.log("  Parent.prototype:", Parent.prototype);
console.log("  Child.prototype[[Prototype]] === Parent.prototype:", Object.getPrototypeOf(Child.prototype) === Parent.prototype);

// Try to manually check if prototype is accessible
const func = Child;
console.log("\nDirect function property access:");
console.log("  typeof func:", typeof func);
console.log("  func.prototype:", func.prototype);

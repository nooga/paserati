// expect: true
// Test lazy prototype initialization
// Functions should not have prototype until accessed or used as constructor

// Regular function - prototype not accessed
function utility(x: number) {
  return x * 2;
}

// Use as regular function
console.log(utility(21)); // expect: 42

// Arrow functions can't be constructors
let arrow = (x: number) => x * 3;
console.log(arrow(14)); // expect: 42

// Constructor function - prototype created on first new
function Widget(name: string) {
  this.name = name;
}

// Create instance - this triggers prototype creation
let w1 = new Widget("gadget");
console.log(w1.name); // expect: gadget
console.log(w1 instanceof Widget); // expect: true

// Now add method to prototype
Widget.prototype.getName = function () {
  return this.name;
};

// Both old and new instances get the method
console.log(w1.getName()); // expect: gadget

let w2 = new Widget("gizmo");
console.log(w2.getName()); // expect: gizmo

// Verify prototype is shared
console.log(w1.getName === w2.getName); // expect: true

true;

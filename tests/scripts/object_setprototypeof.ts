// expect_compile_error: property 'b' does not exist on type { a: number }

// Test basic Object.setPrototypeOf
let obj = { a: 1 };
let proto = { b: 2 };
Object.setPrototypeOf(obj, proto);
let test1 = obj.a === 1 && obj.b === 2;

// Test setting prototype to null
let obj2 = { x: 42 };
Object.setPrototypeOf(obj2, null);
let test2 = Object.getPrototypeOf(obj2) === null;

// Test that setPrototypeOf returns the object
let obj3 = { c: 3 };
let proto2 = { d: 4 };
let result = Object.setPrototypeOf(obj3, proto2);
let test3 = result === obj3 && obj3.d === 4;

// Test prototype chain modification
function Animal() {}
Animal.prototype.speak = function () {
  return "sound";
};
function Dog() {}
let dogInstance = new Dog();
Object.setPrototypeOf(dogInstance, Animal.prototype);
let test4 = dogInstance instanceof Animal && dogInstance.speak() === "sound";

// All tests should pass
test1 && test2 && test3 && test4;

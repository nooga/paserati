// expect: true

// Test basic Object.create with object prototype
let proto = {
  x: 42,
  greet: function () {
    return "Hello";
  },
};
let obj = Object.create(proto);
console.log(obj.x);
console.log(obj.greet());

// Test Object.create with null prototype
let nullProtoObj = Object.create(null);
console.log(nullProtoObj.__proto__);
console.log(Object.getPrototypeOf(nullProtoObj));

// Test that created object doesn't have own properties
console.log(obj.hasOwnProperty("x"));
console.log(proto.hasOwnProperty("x"));

// Test instanceof with Object.create
function Animal() {}
Animal.prototype.speak = function () {
  return "sound";
};
let dog = Object.create(Animal.prototype);
console.log(dog instanceof Animal);
console.log(dog.speak());

// Test prototype chain
let base = { a: 1 };
let child = Object.create(base);
child.b = 2;
let grandchild = Object.create(child);
grandchild.c = 3;

console.log(grandchild.a);
console.log(grandchild.b);
console.log(grandchild.c);
console.log(grandchild.hasOwnProperty("a"));
console.log(grandchild.hasOwnProperty("c"));

// Final statement that returns the expected value
grandchild.hasOwnProperty("c");

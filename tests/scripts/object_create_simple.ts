// expect: 6

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
console.log(Object.getPrototypeOf(nullProtoObj));

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
grandchild.a + grandchild.b + grandchild.c;

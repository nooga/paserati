// expect: true

// Test basic Object.create with object prototype
let proto = {x: 42, greet: function() { return "Hello"; }};
let obj = Object.create(proto);
let test1 = obj.x === 42 && obj.greet() === "Hello";

// Test Object.create with null prototype
let nullProtoObj = Object.create(null);
let test2 = Object.getPrototypeOf(nullProtoObj) === null;
let test3 = nullProtoObj.__proto__ === undefined;

// Test instanceof with Object.create
function Animal() {}
Animal.prototype.speak = function() { return "sound"; };
let dog = Object.create(Animal.prototype);
let test4 = dog instanceof Animal && dog.speak() === "sound";

// Test prototype chain
let base = {a: 1};
let child = Object.create(base);
child.b = 2;
let grandchild = Object.create(child);
grandchild.c = 3;
let test5 = grandchild.a === 1 && grandchild.b === 2 && grandchild.c === 3;

// All tests should pass
test1 && test2 && test3 && test4 && test5;
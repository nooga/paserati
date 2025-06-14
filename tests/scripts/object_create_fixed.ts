// expect: true

// Test basic Object.create with object prototype using any type
let proto: any = {x: 42, greet: function() { return "Hello"; }};
let obj: any = Object.create(proto);
let test1 = obj.x === 42 && obj.greet() === "Hello";

// Test Object.create with null prototype
let nullProtoObj: any = Object.create(null);
let test2 = Object.getPrototypeOf(nullProtoObj) === null;

// Test prototype chain
let base: any = {a: 1};
let child: any = Object.create(base);
child.b = 2;
let grandchild: any = Object.create(child);
grandchild.c = 3;
let test3 = grandchild.a === 1 && grandchild.b === 2 && grandchild.c === 3;

// All tests should pass
test1 && test2 && test3;
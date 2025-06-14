// expect: true

// Test basic Object.create with object prototype using any type
let proto: any = {x: 42, greet: function() { return "Hello"; }};
let obj: any = Object.create(proto);
let test1 = obj.x === 42 && obj.greet() === "Hello";

// Test that created object doesn't have own properties  
let test2 = obj.hasOwnProperty('x') === false;
let test3 = proto.hasOwnProperty('x') === true;

// All tests should pass
test1 && test2 && test3;
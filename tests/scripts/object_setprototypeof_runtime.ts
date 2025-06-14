// expect: true

// Test basic Object.setPrototypeOf
let obj: any = {a: 1};
let proto: any = {b: 2};
Object.setPrototypeOf(obj, proto);
let test1 = obj.a === 1 && obj.b === 2;

// Test setting prototype to null
let obj2: any = {x: 42};
Object.setPrototypeOf(obj2, null);
let test2 = Object.getPrototypeOf(obj2) === null;

// Test that setPrototypeOf returns the object
let obj3: any = {c: 3};
let proto2: any = {d: 4};
let result = Object.setPrototypeOf(obj3, proto2);
let test3 = result === obj3 && obj3.d === 4;

// All tests should pass
test1 && test2 && test3;
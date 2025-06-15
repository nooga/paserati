// Test basic delete operator functionality

// Test 1: Delete property from object
let obj = { x: 10, y: 20 };
console.log(delete obj.x);  // expect: true
console.log(obj.x);         // expect: undefined
console.log(obj.y);         // expect: 20

// Test 2: Delete non-existent property
let obj2 = { a: 1 };
console.log(delete obj2.b); // expect: true

// Test 3: Delete with bracket notation (string literal only for now)
let obj3 = { foo: "bar" };
console.log(delete obj3["foo"]); // expect: true
console.log(obj3.foo);           // expect: undefined

// Test 4: Delete returns boolean
let obj4 = { prop: true };
let result = delete obj4.prop;
console.log(typeof result); // expect: boolean
console.log(result);        // expect: true

// Test 5: Multiple deletes on same object
let obj5 = { a: 1, b: 2, c: 3 };
console.log(delete obj5.a); // expect: true
console.log(delete obj5.b); // expect: true
console.log(obj5.c);        // expect: 3
console.log(obj5.a);        // expect: undefined
console.log(obj5.b);        // expect: undefined

// Test 6: Delete from already converted dict object
let obj6 = { x: 1 };
delete obj6.x;              // This converts to dict
console.log(delete obj6.y); // expect: true (deleting non-existent from dict)

// Test 7: Cannot delete from array (for now)
let arr = [1, 2, 3];
console.log(delete arr.length); // expect: false
console.log(arr.length);        // expect: 3
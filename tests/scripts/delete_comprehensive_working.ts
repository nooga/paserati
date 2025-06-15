// expect: true

// Comprehensive test of delete operator functionality

// Test 1: Basic deletion
let obj1 = { x: 10, y: 20 };
let delete1 = delete obj1.x;
let after1 = obj1.x === undefined;
let remaining1 = obj1.y === 20;

// Test 2: Delete non-existent property (should return true)
let obj2: { a: number; nonexistent?: number } = { a: 1 };
let delete2 = delete obj2.nonexistent;

// Test 3: Multiple operations on same object
let obj3 = { a: 1, b: 2, c: 3 };
let deleteA = delete obj3.a;
let deleteB = delete obj3.b;
let afterA = obj3.a === undefined;
let afterB = obj3.b === undefined;
let remainingC = obj3.c === 3;

// Test 4: Delete from object that gets passed around
function testDelete(o: any) {
  return delete o.prop;
}
let obj4 = { prop: "value", other: "data" };
let deleteResult = testDelete(obj4);
let propGone = obj4.prop === undefined;
let otherRemains = obj4.other === "data";

// Test 5: Multiple references to same object
let obj5 = { x: 100 };
let ref5 = obj5; // Same object reference
let deleteFromRef = delete ref5.x;
let originalAlsoDeleted = obj5.x === undefined;

// All tests should pass
delete1 &&
  after1 &&
  remaining1 && // Basic deletion works
  delete2 && // Non-existent property returns true
  deleteA &&
  deleteB &&
  afterA &&
  afterB &&
  remainingC && // Multiple deletions
  deleteResult &&
  propGone &&
  otherRemains && // Function parameter deletion
  deleteFromRef &&
  originalAlsoDeleted; // Reference semantics work

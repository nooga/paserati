// expect: true

// Comprehensive test of delete operator functionality

// Test 1: Basic deletion
let obj1 = { x: 10, y: 20 };
let delete1 = delete obj1.x;
let after1 = obj1.x === undefined;
let remaining1 = obj1.y === 20;

// Test 2: Multiple operations on same object
let obj2 = { a: 1, b: 2, c: 3 };
let deleteA = delete obj2.a;
let deleteB = delete obj2.b;
let afterA = obj2.a === undefined;
let afterB = obj2.b === undefined;
let remainingC = obj2.c === 3;

// Test 3: Multiple references to same object (ersatz solution test!)
let obj3 = { x: 100 };
let ref3 = obj3;  // Same object reference
let deleteFromRef = delete ref3.x;
let originalAlsoDeleted = obj3.x === undefined;  // This tests the ersatz fix!

// All tests should pass
delete1 && after1 && remaining1 &&  // Basic deletion works
deleteA && deleteB && afterA && afterB && remainingC &&  // Multiple deletions
deleteFromRef && originalAlsoDeleted;  // ERSATZ SOLUTION: Reference semantics work!
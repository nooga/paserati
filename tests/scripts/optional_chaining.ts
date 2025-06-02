// Test file for optional chaining (?.) operator
// expect: deep

// Test 1: Basic optional chaining with existing property
let obj1 = { name: "Alice", age: 30 };
console.log("Test 1 - existing property:", obj1?.name); // Should print "Alice"

// Test 2: Optional chaining with missing property
console.log("Test 2 - missing property:", obj1?.missing); // Should print undefined

// Test 3: Optional chaining with null object
let obj2 = null;
console.log("Test 3 - null object:", obj2?.prop); // Should print undefined

// Test 4: Optional chaining with undefined object
let obj3 = undefined;
console.log("Test 4 - undefined object:", obj3?.prop); // Should print undefined

// Test 5: Optional chaining with array
let arr = [1, 2, 3];
console.log("Test 5 - array length:", arr?.length); // Should print 3

// Test 6: Optional chaining with string
let str = "hello";
console.log("Test 6 - string length:", str?.length); // Should print 5

// Test 7: Optional chaining with null array
let nullArr = null;
console.log("Test 7 - null array length:", nullArr?.length); // Should print undefined

// Test 8: Optional chaining in assignment
let result = obj1?.name;
console.log("Test 8 - assignment:", result); // Should print "Alice"

// Test 9: Optional chaining with function result
function getObj() {
  return { prop: "function result" };
}
console.log("Test 9 - function result:", getObj()?.prop); // Should print "function result"

// Test 10: Compare regular vs optional access
let comparison = obj1.name === obj1?.name;
console.log("Test 10 - regular vs optional:", comparison); // Should print true

console.log("Optional chaining tests completed!");

// Test 11: Chained optional chaining - SUCCESS case
let nestedObj = { level1: { level2: { value: "deep" } } };
console.log("Test 11 - chained success:", nestedObj?.level1?.level2?.value); // Should print "deep"

// Test 12: Chained optional chaining - NULL/UNDEFINED case
let nullObj2 = null;
console.log("Test 12 - chained null:", nullObj2?.level1?.level2?.value); // Should print undefined

// Test 13: Chained optional chaining - MISSING property case
console.log("Test 13 - chained missing:", nestedObj?.missing?.level2?.value); // Should print undefined

console.log("All optional chaining tests completed!");

// Final expression to be evaluated - testing chained optional chaining
nestedObj?.level1?.level2?.value;

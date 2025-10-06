// expect: all tests passed
// Test that Map and Set objects are truthy in boolean contexts
// This is a regression test for a bug where Map and Set were incorrectly falsy

const m = new Map();
const s = new Set();

// Test Map truthiness in if statement
if (!m) {
  console.log("FAIL: Map should be truthy");
}

// Test Set truthiness in if statement
if (!s) {
  console.log("FAIL: Set should be truthy");
}

// Test Map in boolean NOT operator
if (!m === true) {
  console.log("FAIL: !Map should be false");
}

// Test Set in boolean NOT operator
if (!s === true) {
  console.log("FAIL: !Set should be false");
}

// Test Map in ternary operator
const mapResult = m ? "truthy" : "falsy";
if (mapResult !== "truthy") {
  console.log("FAIL: Map should be truthy in ternary");
}

// Test Set in ternary operator
const setResult = s ? "truthy" : "falsy";
if (setResult !== "truthy") {
  console.log("FAIL: Set should be truthy in ternary");
}

// Test with non-empty Map
const m2 = new Map();
m2.set("key", "value");
if (!m2) {
  console.log("FAIL: Non-empty Map should be truthy");
}

// Test with non-empty Set
const s2 = new Set();
s2.add(1);
if (!s2) {
  console.log("FAIL: Non-empty Set should be truthy");
}

// Test Set.forEach works
let sum = 0;
const testSet = new Set<number>();
testSet.add(1);
testSet.add(2);
testSet.add(3);

testSet.forEach((val) => {
  sum += val;
});

if (sum !== 6) {
  console.log("FAIL: Set.forEach should work");
}

"all tests passed";

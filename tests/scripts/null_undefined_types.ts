// expect: 4

// 1. Literals
let x = undefined;
let y = null;

// 2. Type Annotations
let xTyped: undefined;
let yTyped: null;

// 3. Assign literal to correctly typed variable
xTyped = undefined;
yTyped = null;

// 4. Assign literal to variable inferred from literal
let xInferred = undefined;
let yInferred = null;
xInferred = undefined; // Should be okay (undefined = undefined)
yInferred = null; // Should be okay (null = null)

// 5. Invalid Assignments (Expect Type Errors Here)
// xTyped = null;       // Error: Cannot assign null to undefined
// yTyped = undefined;  // Error: Cannot assign undefined to null
// xInferred = null;    // Error: Cannot assign null to undefined
// yInferred = undefined; // Error: Cannot assign undefined to null

// 6. Function parameter and return types
function processNull(p: null): null {
  return p;
}

function processUndefined(p: undefined): undefined {
  return undefined;
}

let nullResult = processNull(null);
let undefinedResult = processUndefined(undefined);

// 7. Arrow function parameter and return types
const arrowNull = (p: null): null => p;
const arrowUndefined = (p: undefined): undefined => undefined;

let arrowNullResult = arrowNull(null);
let arrowUndefinedResult = arrowUndefined(undefined);

let expected = 0;

// 8. Checking values (Runtime check, type checker should allow)
if (x === undefined) {
  expected++;
}
if (y === null) {
  expected++;
}
if (xTyped === undefined) {
  expected++;
}
if (yTyped === null) {
  expected++;
}

expected;

// TODO: Add tests where null/undefined are assigned to 'any' or 'unknown'
// TODO: Add tests assigning other types to null/undefined typed vars (should fail)

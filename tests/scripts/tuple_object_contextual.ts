// Test contextual typing for tuples inside object literals
// expect: world

// Function with object parameter containing tuple
function takeObj(obj: { point: [number, string] }): string {
  return obj.point[1];
}

// Object literal with array should get tuple type from context
let r = takeObj({ point: [123, "world"] });
r;

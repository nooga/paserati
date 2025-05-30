// Test polymorphic inline cache with different object shapes

let objA = { x: 10, y: 20 }; // Shape 1
let objB = { x: 30, y: 40, z: 50 }; // Shape 2 (different!)

let result = 0;

// Access the same property on objects with different shapes
// This should trigger polymorphic caching
for (let i = 0; i < 3; i++) {
  result = result + objA.x; // Shape 1
  result = result + objB.x; // Shape 2 - should make cache polymorphic
}

result;

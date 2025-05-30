// Test inline cache performance with different object patterns

// Create objects with same shape (monomorphic case)
let obj1 = { x: 10, y: 20 };
let obj2 = { x: 30, y: 40 };

// Multiple accesses to same property (should hit cache)
let sum = 0;
sum = sum + obj1.x; // First access, cache miss
sum = sum + obj1.x; // Second access, cache hit!
sum = sum + obj2.x; // Same shape, cache hit!

// Different property
sum = sum + obj1.y; // Different property, might miss then cache

// Create object with different shape (polymorphic case)
let obj3 = { x: 50, y: 60, z: 70 };

// Access same property on different shape
sum = sum + obj3.x; // Different shape, triggers polymorphic cache

sum;

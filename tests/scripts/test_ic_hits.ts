// expect: 50
// Test inline cache with repeated access (should show cache hits)

let obj = { x: 10, y: 20 };
let total = 0;

// Loop to create multiple accesses to the same property
for (let i = 0; i < 5; i++) {
  total = total + obj.x; // First iteration: miss, subsequent: hits!
}

total;

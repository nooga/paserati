// expect: ok

// ============================================================================
// 8. Rest Parameters in Methods
// ============================================================================

console.log("\n--- Rest Parameters in Methods ---");

// Object with variadic methods
let calculator = {
  add: function (...nums: number[]): number {
    let sum = 0;
    for (let i = 0; i < nums.length; i++) {
      sum += nums[i];
    }
    return sum;
  },

  multiply: (...nums: number[]) => {
    let product = 1;
    for (let i = 0; i < nums.length; i++) {
      product *= nums[i];
    }
    return product;
  },
};

console.log("calculator.add(1, 2, 3):", calculator.add(1, 2, 3));
console.log("calculator.multiply(2, 3, 4):", calculator.multiply(2, 3, 4));

("ok");

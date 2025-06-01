// Test shorthand method syntax
// expect: 5
const mathUtils = {
  add(a: number, b: number): number {
    return a + b;
  },

  multiply(x: number, y: number): number {
    return x * y;
  },

  // Mix with regular property
  PI: 3.14159,

  // More complex method
  calculateArea(radius: number): number {
    return mathUtils.PI * mathUtils.multiply(radius, radius);
  },
};

// Test simple method calls
mathUtils.add(2, 3);

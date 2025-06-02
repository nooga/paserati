// expect: 200

// Test trailing comma with shorthand
let a = 10;
let b = 20;
let obj1 = { a, b };
let result = obj1.a + obj1.b; // 30

// Test mixing shorthand and regular properties
let name = "test";
let obj2 = {
  name,
  value: 50,
  active: true,
};
result += obj2.value; // 80

// Test shorthand in nested objects
let x = 5;
let y = 15;
let nested = {
  point: { x, y },
  metadata: { x, name: "point" },
};
result += nested.point.x + nested.point.y; // 100
result += nested.metadata.x; // 105

// Test shorthand with computed values
let computed = 25;
let dynamic = {
  computed,
  doubled: computed * 2,
};
result += dynamic.computed + dynamic.doubled; // 180

// Test shorthand with function values
function getValue() {
  return 10;
}
let funcValue = getValue();
let withFunc = { funcValue };
result += withFunc.funcValue; // 190

// Test empty shorthand object (edge case)
// let empty = {}; // This should still work

// Final increment to reach 200
result += 10; // 200

result;

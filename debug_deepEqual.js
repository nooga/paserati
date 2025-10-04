console.log("Testing deepEqual:");
var result1 = assert.deepEqual(
  { a: { x: 1 }, b: [true] },
  { a: { x: 1 }, b: [true] }
);
console.log("Equal objects result:", result1);

try {
  assert.deepEqual({}, { a: { x: 1 }, b: [true] });
  console.log("Should not reach here");
} catch (e) {
  console.log("Caught error:", e);
}

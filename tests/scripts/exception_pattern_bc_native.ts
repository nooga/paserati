// expect: true
// Test bc -> native exception pattern
let caught = false;
try {
  JSON.parse("{invalid json}");
} catch(e) {
  console.log("Caught native error:", e.message);
  caught = true;
}
caught;
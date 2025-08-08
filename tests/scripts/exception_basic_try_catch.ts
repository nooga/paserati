// expect: true
// Test basic try/catch with throw statement
let caught = false;
try {
  throw new Error("test error");
} catch(e) {
  console.log("Caught:", e.message);
  caught = true;
}
caught;
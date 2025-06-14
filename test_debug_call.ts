// Minimal test to debug call issue
console.log("=== Script start ===");

function test() {
  console.log("In test function");
}

console.log("About to call test.call");
test.call(null);
console.log("=== Script end ===");